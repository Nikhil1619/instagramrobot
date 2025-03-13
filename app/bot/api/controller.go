package api

import (
	"errors"
	"fmt"
	"regexp"

	"gopkg.in/telebot.v4"

	"github.com/omegaatt36/instagramrobot/appmodule/instagram"
	"github.com/omegaatt36/instagramrobot/appmodule/providers"
	"github.com/omegaatt36/instagramrobot/appmodule/telegram"
	"github.com/omegaatt36/instagramrobot/appmodule/threads"
	"github.com/omegaatt36/instagramrobot/logging"
)

// Controller is the main controller for the bot.
type Controller struct {
	bot       *telebot.Bot // Bot instance
	urlParser *regexp.Regexp
}

// NewController creates a new Controller instance.
func NewController(b *telebot.Bot) *Controller {
	regex := `(http|ftp|https)://([\w_-]+(?:(?:\.[\w_-]+)+))([\w.,@@?^=%&:/~+#-]*[\w@?^=%&/~+#-])?`
	r := regexp.MustCompile(regex)
	return &Controller{bot: b, urlParser: r}
}

// OnStart is the entry point for the incoming update
func (*Controller) OnStart(c telebot.Context) error {
	// Ignore channels and groups
	if c.Chat().Type != telebot.ChatPrivate {
		return nil
	}

	if err := c.Reply("Hello! I'm Instagram keeper. Please send me some Instagram public post/reels."); err != nil {
		return fmt.Errorf("couldn't sent the start command response: %w", err)
	}

	return nil
}

func (x *Controller) extractLinksFromString(input string) []string {
	return x.urlParser.FindAllString(input, -1)
}

func (x *Controller) OnText(c telebot.Context) error {
	// Get the required channel ID from the config
	requiredChannelID := int64(-1002108741045) // Replace with your channel ID

	// Check if the user is in the required channel
	isInChannel, err := x.isUserInChannel(c, requiredChannelID)
	if err != nil {
		logging.Error(err)
		return x.replyError(c, "Error checking subscription status.")
	}

	if !isInChannel {
		return x.promptSubscription(c)
	}

	links := x.extractLinksFromString(c.Message().Text)

	// Send proper error if text has no link inside
	if len(links) == 0 {
		if c.Chat().Type != telebot.ChatPrivate {
			return nil
		}

		err := errors.New("Invalid command,\nPlease send the Instagram post link.")
		logging.Error(fmt.Errorf("OnText.replyError: %w", err))
		return x.replyError(c, "Invalid command,\nPlease send the Instagram post link.")
	}

	if err := x.processLinks(links, c.Message()); err != nil {
		if c.Chat().Type != telebot.ChatPrivate {
			return nil
		}

		logging.Error(fmt.Errorf("OnText.processLinks: %w", err))
		return x.replyError(c, err.Error())
	}

	return nil
}

// isUserInChannel checks if the user is in the required channel
func (x *Controller) isUserInChannel(c telebot.Context, requiredChannelID int64) (bool, error) {
	channel := &telebot.Chat{ID: requiredChannelID}
	member, err := c.Bot().ChatMemberOf(channel, c.Sender())
	if err != nil {
		return false, fmt.Errorf("error checking subscription status: %w", err)
	}
	return member.Role == telebot.Member || member.Role == telebot.Administrator || member.Role == telebot.Creator, nil
}

// promptSubscription prompts the user to subscribe to the required channel
func (*Controller) promptSubscription(c telebot.Context) error {
	message := "🚨 To use this bot, you need to join our channel: @ThisDeal" // Channel username
	_, err := c.Bot().Send(c.Sender(), message)
	if err != nil {
		return fmt.Errorf("couldn't prompt for subscription: %w", err)
	}
	return nil
}

// Gets list of links from user message text
// and processes each one of them one by one.
func (x *Controller) processLinks(links []string, m *telebot.Message) error {
	const maxLinksPerMessage = 3

	linkProcessor := providers.NewLinkProcessor(providers.NewLinkProcessorRequest{
		InstagramFetcher: instagram.NewInstagramFetcher(),
		ThreadsFetcher:   threads.NewExtractor(),
		Sender:           telegram.NewMediaSender(x.bot, m),
	})

	for index, link := range links {
		if index == maxLinksPerMessage {
			logging.Errorf("can't process more than %c links per message", maxLinksPerMessage)
			break
		}

		if err := linkProcessor.ProcessLink(link); err != nil {
			logging.Error(fmt.Errorf("processLinks.ProcessLink: %w", err))
			continue // 繼續處理下一個 link
		}
	}

	return nil
}

func (*Controller) replyError(c telebot.Context, text string) error {
	_, err := c.Bot().Reply(c.Message(), fmt.Sprintf("⚠️ *Oops, ERROR!*\n\n`%v`", text), telebot.ModeMarkdown)
	if err != nil {
		return fmt.Errorf("couldn't reply the Error, chat_id: %d, err: %w", c.Chat().ID, err)
	}

	return nil
}