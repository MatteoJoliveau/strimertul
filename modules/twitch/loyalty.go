package twitch

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	irc "github.com/gempir/go-twitch-irc/v2"
	"github.com/nicklaw5/helix"
	"github.com/strimertul/strimertul/modules/loyalty"
)

func (b *Bot) SetupLoyalty(loyalty *loyalty.Manager) {
	config := loyalty.Config()
	b.Loyalty = loyalty
	b.SetBanList(config.BanList)

	// Add loyalty-based commands
	commands["!redeem"] = BotCommand{
		Description: "Redeem a reward with loyalty points",
		Usage:       "!redeem <reward-id> [request text]",
		AccessLevel: ALTEveryone,
		Handler:     cmdRedeemReward,
	}
	commands["!balance"] = BotCommand{
		Description: "See your current point balance",
		Usage:       "!balance",
		AccessLevel: ALTEveryone,
		Handler:     cmdBalance,
	}
	commands["!goals"] = BotCommand{
		Description: "Check currently active community goals",
		Usage:       "!goals",
		AccessLevel: ALTEveryone,
		Handler:     cmdGoalList,
	}
	commands["!contribute"] = BotCommand{
		Description: "Contribute points to a community goal",
		Usage:       "!contribute <points> [<goal-id>]",
		AccessLevel: ALTEveryone,
		Handler:     cmdContributeGoal,
	}

	// Setup handler for adding points over time
	b.Client.OnConnect(func() {
		if config.Points.Interval > 0 {
			go func() {
				b.logger.Info("loyalty poll started")
				for {
					// Wait for next poll
					time.Sleep(time.Duration(config.Points.Interval) * time.Second)

					// Check if streamer is online, if possible
					streamOnline := true
					status, err := b.api.API.GetStreams(&helix.StreamsParams{
						UserLogins: []string{b.config.Channel},
					})
					if err != nil {
						b.logger.WithError(err).Error("Error checking stream status")
					} else {
						streamOnline = len(status.Data.Streams) > 0
					}

					// If stream is confirmed offline, don't give points away!
					if !streamOnline {
						b.logger.Debug("loyalty poll active but stream is offline!")
						continue
					} else {
						b.logger.Debug("awarding points")
					}

					// Get user list
					users, err := b.Client.Userlist(b.config.Channel)
					if err != nil {
						b.logger.WithError(err).Error("error listing users")
						continue
					}

					// Iterate for each user in the list
					pointsToGive := make(map[string]int64)
					for _, user := range users {
						// Check if user is blocked
						if b.IsBanned(user) {
							continue
						}

						// Check if user was active (chatting) for the bonus dingus
						award := config.Points.Amount
						if b.IsActive(user) {
							award += config.Points.ActivityBonus
						}

						// Add to point pool if already on it, otherwise initialize
						pointsToGive[user] = award
					}

					b.ResetActivity()

					// If changes were made, save the pool!
					if len(users) > 0 {
						b.Loyalty.GivePoints(pointsToGive)
					}
				}
			}()
		}
	})
}

func (b *Bot) SetBanList(banned []string) {
	b.banlist = make(map[string]bool)
	for _, usr := range banned {
		b.banlist[usr] = true
	}
}

func (b *Bot) IsBanned(user string) bool {
	banned, ok := b.banlist[user]
	return ok && banned
}

func (b *Bot) IsActive(user string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	active, ok := b.activeUsers[user]
	return ok && active
}

func (b *Bot) ResetActivity() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activeUsers = make(map[string]bool)
}

func cmdBalance(bot *Bot, message irc.PrivateMessage) {
	// Get user balance
	balance := bot.Loyalty.GetPoints(message.User.Name)
	bot.Client.Say(message.Channel, fmt.Sprintf("%s: You have %d %s!", message.User.DisplayName, balance, bot.Loyalty.Config().Currency))
}

func cmdRedeemReward(bot *Bot, message irc.PrivateMessage) {
	parts := strings.Fields(message.Message)
	if len(parts) < 2 {
		return
	}
	redeemID := parts[1]

	// Find reward
	reward := bot.Loyalty.GetReward(redeemID)
	if reward.ID == "" {
		return
	}

	// Reward not active, return early
	if !reward.Enabled {
		return
	}

	// Get user balance
	balance := bot.Loyalty.GetPoints(message.User.Name)
	config := bot.Loyalty.Config()

	// Check if user can afford the reward
	if balance-reward.Price < 0 {
		bot.Client.Say(message.Channel, fmt.Sprintf("I'm sorry %s but you cannot afford this (have %d %s, need %d)", message.User.DisplayName, balance, config.Currency, reward.Price))
		return
	}

	text := ""
	if len(parts) > 2 {
		text = strings.Join(parts[2:], " ")
	}

	// Perform redeem
	if err := bot.Loyalty.PerformRedeem(loyalty.Redeem{
		Username:    message.User.Name,
		DisplayName: message.User.DisplayName,
		When:        time.Now(),
		Reward:      reward,
		RequestText: text,
	}); err != nil {
		switch err {
		case loyalty.ErrRedeemInCooldown:
			nextAvailable := bot.Loyalty.GetRewardCooldown(reward.ID)
			bot.Client.Say(message.Channel, fmt.Sprintf("%s: That reward is in cooldown (available in %s)", message.User.DisplayName,
				time.Until(nextAvailable).Truncate(time.Second)))
		default:
			bot.logger.WithError(err).Error("error while performing redeem")
		}
		return
	}

	bot.Client.Say(message.Channel, fmt.Sprintf("HolidayPresent %s has redeemed %s! (new balance: %d %s)", message.User.DisplayName, reward.Name, bot.Loyalty.GetPoints(message.User.Name), config.Currency))
}

func cmdGoalList(bot *Bot, message irc.PrivateMessage) {
	goals := bot.Loyalty.Goals()
	if len(goals) < 1 {
		bot.Client.Say(message.Channel, fmt.Sprintf("%s: There are no active community goals right now :(!", message.User.DisplayName))
		return
	}
	msg := "Current goals: "
	for _, goal := range goals {
		if !goal.Enabled {
			continue
		}
		msg += fmt.Sprintf("%s (%d/%d %s) [id: %s] | ", goal.Name, goal.Contributed, goal.TotalGoal, bot.Loyalty.Config().Currency, goal.ID)
	}
	msg += " Contribute with <!contribute POINTS GOALID>"
	bot.Client.Say(message.Channel, msg)
}

func cmdContributeGoal(bot *Bot, message irc.PrivateMessage) {
	goals := bot.Loyalty.Goals()

	// Set defaults if user doesn't provide them
	points := int64(100)
	goalIndex := -1
	hasGoals := false

	// Get first unreached goal for default
	for index, goal := range goals {
		if !goal.Enabled {
			continue
		}
		hasGoals = true
		if goal.Contributed < goal.TotalGoal {
			goalIndex = index
			break
		}
	}

	// Do we not have any goal we can contribute to? Hooray I guess?
	if goalIndex < 0 {
		if hasGoals {
			bot.Client.Say(message.Channel, fmt.Sprintf("%s: All active community goals have been reached already! ShowOfHands", message.User.DisplayName))
		} else {
			bot.Client.Say(message.Channel, fmt.Sprintf("%s: There are no active community goals right now :(!", message.User.DisplayName))
		}
		return
	}

	// Parse parameters if provided
	parts := strings.Fields(message.Message)
	if len(parts) > 1 {
		newpoints, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil {
			if newpoints <= 0 {
				bot.Client.Say(message.Channel, fmt.Sprintf("Nice try %s SoBayed", message.User.DisplayName))
				return
			}
			points = newpoints
		}
		if len(parts) > 2 {
			found := false
			goalID := parts[2]
			// Find Goal index
			for index, goal := range goals {
				if !goal.Enabled {
					continue
				}
				if goal.ID == goalID {
					goalIndex = index
					found = true
					break
				}
			}
			// Invalid goal ID provided
			if !found {
				bot.Client.Say(message.Channel, fmt.Sprintf("%s: I couldn't find that goal ID :(", message.User.DisplayName))
				return
			}
		}
	}

	// Get goal
	selectedGoal := goals[goalIndex]

	// Check if goal was reached already
	if selectedGoal.Contributed >= selectedGoal.TotalGoal {
		bot.Client.Say(message.Channel, fmt.Sprintf("%s: This goal was already reached! ヾ(•ω•`)o", message.User.DisplayName))
		return
	}

	// Add points to goal
	if err := bot.Loyalty.PerformContribution(selectedGoal, message.User.Name, points); err != nil {
		bot.logger.WithError(err).Error("error while contributing to goal")
		return
	}

	config := bot.Loyalty.Config()
	newRemaining := selectedGoal.TotalGoal - selectedGoal.Contributed
	bot.Client.Say(message.Channel, fmt.Sprintf("ShowOfHands %s contributed %d %s to \"%s\"!! Only %d %s left!", message.User.DisplayName, points, config.Currency, selectedGoal.Name, newRemaining, config.Currency))

	// Check if goal was reached!
	// TODO Replace this with sub from loyalty system or something?
	if newRemaining <= 0 {
		bot.Client.Say(message.Channel, fmt.Sprintf("ShowOfHands The community goal \"%s\" was reached! ShowOfHands", selectedGoal.Name))
	}
}