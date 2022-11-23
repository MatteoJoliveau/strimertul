package twitch

import (
	"errors"
	"fmt"
	"time"

	"git.sr.ht/~hamcha/containers"
	lru "github.com/hashicorp/golang-lru"
	jsoniter "github.com/json-iterator/go"
	"github.com/nicklaw5/helix/v2"
	"github.com/strimertul/strimertul/modules/database"
	"go.uber.org/zap"

	"github.com/strimertul/strimertul/modules"
	"github.com/strimertul/strimertul/modules/loyalty"
)

var json = jsoniter.ConfigFastest

type Client struct {
	Config     Config
	Bot        *Bot
	db         *database.DBModule
	API        *helix.Client
	logger     *zap.Logger
	manager    *modules.Manager
	eventCache *lru.Cache

	restart      chan bool
	streamOnline *containers.RWSync[bool]
}

func Register(manager *modules.Manager) error {
	db, ok := manager.Modules["db"].(*database.DBModule)
	if !ok {
		return errors.New("db module not found")
	}

	logger := manager.Logger(modules.ModuleTwitch)

	eventCache, err := lru.New(128)
	if err != nil {
		return fmt.Errorf("could not create LRU cache for events: %w", err)
	}

	// Get Twitch config
	var config Config
	err = db.GetJSON(ConfigKey, &config)
	if err != nil {
		if !errors.Is(err, database.ErrEmptyKey) {
			return fmt.Errorf("failed to get twitch config: %w", err)
		}
		config.Enabled = false
	}

	// Create Twitch client
	client := &Client{
		Config:       config,
		db:           db,
		logger:       logger,
		restart:      make(chan bool, 128),
		streamOnline: containers.NewRWSync(false),
		manager:      manager,
		eventCache:   eventCache,
	}

	// Listen for config changes
	go db.Subscribe(func(key, value string) {
		switch key {
		case ConfigKey:
			err := json.UnmarshalFromString(value, &config)
			if err != nil {
				logger.Error("failed to unmarshal config", zap.Error(err))
				return
			}
			api, err := client.getHelixAPI()
			if err != nil {
				logger.Warn("failed to create new twitch client, keeping old credentials", zap.Error(err))
				return
			}
			client.API = api

			logger.Info("reloaded/updated Twitch API")
		case BotConfigKey:
			var twitchBotConfig BotConfig
			err := json.UnmarshalFromString(value, &twitchBotConfig)
			if err != nil {
				logger.Error("failed to unmarshal config", zap.Error(err))
				return
			}
			err = client.Bot.Client.Disconnect()
			if err != nil {
				logger.Warn("failed to disconnect from Twitch IRC", zap.Error(err))
			}
			if client.Config.EnableBot {
				if err := client.startBot(manager); err != nil {
					if !errors.Is(err, database.ErrEmptyKey) {
						logger.Error("failed to re-create bot", zap.Error(err))
					}
				}
			}
			client.restart <- true
			logger.Info("reloaded/restarted Twitch bot")
		}
	}, ConfigKey, BotConfigKey)

	if config.Enabled {
		client.API, err = client.getHelixAPI()
		if err != nil {
			client.logger.Error("failed to create twitch client", zap.Error(err))
		}
	}

	if client.Config.EnableBot {
		if err := client.startBot(manager); err != nil {
			if !errors.Is(err, database.ErrEmptyKey) {
				return err
			}
		}
	}

	go client.runStatusPoll()
	go client.connectWebsocket()

	go func() {
		for {
			if client.Config.EnableBot && client.Bot != nil {
				err := client.RunBot()
				if err != nil {
					logger.Error("failed to connect to Twitch IRC", zap.Error(err))
					// Wait for config change before retrying
					<-client.restart
				}
			} else {
				<-client.restart
			}
		}
	}()

	manager.Modules[modules.ModuleTwitch] = client

	// If loyalty module is enabled, set-up loyalty commands
	if loyaltyManager, ok := client.manager.Modules[modules.ModuleLoyalty].(*loyalty.Manager); ok && client.Bot != nil {
		client.Bot.SetupLoyalty(loyaltyManager)
	}

	return nil
}

func (c *Client) runStatusPoll() {
	c.logger.Info("status poll started")
	for {
		// Wait for next poll
		time.Sleep(60 * time.Second)

		// Make sure we're configured and connected properly first
		if !c.Config.Enabled || c.Bot == nil || c.Bot.config.Channel == "" {
			continue
		}

		// Check if streamer is online, if possible
		func() {
			status, err := c.API.GetStreams(&helix.StreamsParams{
				UserLogins: []string{c.Bot.config.Channel}, // TODO Replace with something non bot dependant
			})
			if err != nil {
				c.logger.Error("Error checking stream status", zap.Error(err))
			} else {
				c.streamOnline.Set(len(status.Data.Streams) > 0)
			}

			err = c.db.PutJSON(StreamInfoKey, status.Data.Streams)
			if err != nil {
				c.logger.Warn("Error saving stream info", zap.Error(err))
			}
		}()
	}
}

func (c *Client) startBot(manager *modules.Manager) error {
	// Get Twitch bot config
	var twitchBotConfig BotConfig
	err := c.db.GetJSON(BotConfigKey, &twitchBotConfig)
	if err != nil {
		if !errors.Is(err, database.ErrEmptyKey) {
			return fmt.Errorf("failed to get bot config: %w", err)
		}
		c.Config.EnableBot = false
	}

	// Create and run IRC bot
	c.Bot = NewBot(c, twitchBotConfig)

	// If loyalty module is enabled, set-up loyalty commands
	if loyaltyManager, ok := manager.Modules[modules.ModuleLoyalty].(*loyalty.Manager); ok && c.Bot != nil {
		c.Bot.SetupLoyalty(loyaltyManager)
	}

	return nil
}

func (c *Client) getHelixAPI() (*helix.Client, error) {
	redirectURI, err := c.getRedirectURI()
	if err != nil {
		return nil, err
	}

	// Create Twitch client
	api, err := helix.NewClient(&helix.Options{
		ClientID:     c.Config.APIClientID,
		ClientSecret: c.Config.APIClientSecret,
		RedirectURI:  redirectURI,
	})
	if err != nil {
		return nil, err
	}

	// Get access token
	resp, err := api.RequestAppAccessToken([]string{"user:read:email"})
	if err != nil {
		return nil, err
	}
	// Set the access token on the client
	api.SetAppAccessToken(resp.Data.AccessToken)

	return api, nil
}

func (c *Client) RunBot() error {
	cherr := make(chan error)
	go func() {
		cherr <- c.Bot.Connect()
	}()
	select {
	case <-c.restart:
		return nil
	case err := <-cherr:
		return err
	}
}

func (c *Client) Status() modules.ModuleStatus {
	if !c.Config.Enabled {
		return modules.ModuleStatus{
			Enabled: false,
		}
	}

	return modules.ModuleStatus{
		Enabled:      true,
		Working:      c.Bot != nil && c.Bot.Client != nil,
		Data:         struct{}{},
		StatusString: "",
	}
}

func (c *Client) Close() error {
	return c.Bot.Client.Disconnect()
}
