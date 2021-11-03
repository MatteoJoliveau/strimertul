package twitch

import "github.com/strimertul/strimertul/database"

const BotModulesConfigKey = "twitch/bot-modules/config"

type BotModulesConfig struct {
	EnableTimers bool `json:"enable_timers"`
}

func (b *Bot) LoadModules() error {
	var cfg BotModulesConfig
	err := b.api.db.GetJSON(BotModulesConfigKey, &cfg)
	if err != nil {
		if err != database.ErrKeyNotFound {
			return err
		}
		cfg = BotModulesConfig{
			EnableTimers: false,
		}
	}
	if cfg.EnableTimers {
		b.Timers = SetupTimers(b)
	}
	return nil
}
