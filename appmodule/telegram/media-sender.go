package telegram

import (
	"fmt"
	"regexp"

	"gopkg.in/telebot.v4"

	"github.com/omegaatt36/instagramrobot/domain"
	"github.com/omegaatt36/instagramrobot/logging"
)

type MediaSender struct {
	bot *telebot.Bot
	msg *telebot.Message
}

// NewMediaSender creates a new MediaSender instance
func NewMediaSender(bot *telebot.Bot, msg *telebot.Message) domain.MediaSender {
	return &MediaSender{
		bot: bot,
		msg: msg,
	}
}

// Send will start to process Media and eventually send it to the Telegram chat
func (m *MediaSender) Send(media *domain.Media) error {
	logging.Infof("chatID(%d) source(%s) short code(%s)", m.msg.Sender.ID, media.Source, media.ShortCode)

	var fnSend func(*domain.Media) error

	// Check if media has no child item
	if len(media.Items) == 0 {
		fnSend = m.sendSingleMedia
	} else {
		fnSend = m.sendNestedMedia
	}

	// Execute the media send function
	if err := fnSend(media); err != nil {
		// Return the error if both media and document sending fails
		return fmt.Errorf("failed to send media: %w", err)
	}

	m.SendCaption(media)
	// Send the custom thank you message after successfully sending media
	return m.sendCustomMessage("Share @ThisDeal with your friends") // Customize your message here
}

func (m *MediaSender) sendSingleMedia(media *domain.Media) error {
	if media.URL == "" {
		return nil
	}

	mediaToSend := convertMediaToInputtable(media)

	// Step 1: Attempt to send the media (photo/video)
	if _, err := m.bot.Send(m.msg.Chat, mediaToSend); err != nil {
		logging.Errorf("couldn't send the %s media normally: %v", mediaToSend.MediaType(), err)

		// Step 2: Attempt to send as a document if media send fails
		if err := m.sendAsDocument(media); err != nil {
			return fmt.Errorf("failed to send as document: %w", err)
		}
	}

	return nil // Successfully sent media (if applicable)
}

func (m *MediaSender) sendNestedMedia(media *domain.Media) error {
	const albumLimit = 10
	var album telebot.Album

	// Break down the media items into batches of up to 10
	for i := 0; i < len(media.Items); i += albumLimit {
		// Get the next batch of 10 (or fewer) media items
		end := i + albumLimit
		if end > len(media.Items) {
			end = len(media.Items)
		}

		// Prepare the current batch
		album = nil
		for _, mediaItem := range media.Items[i:end] {
			album = append(album, convertMediaItemToInputtable(mediaItem))
		}

		// Step 1: Try sending the album for the current batch
		if _, err := m.bot.SendAlbum(m.msg.Chat, album); err != nil {
			logging.Errorf("couldn't send the nested media album, attempting to send as document: %v", err)

			// Attempt to send each media item as a document if album sending fails
			for _, mediaItem := range media.Items[i:end] {
				if err := m.sendAsDocument(&domain.Media{
					URL:      mediaItem.URL,
					ShortCode: media.ShortCode, // Optional: Use same shortcode or generate new
				}); err != nil {
					return fmt.Errorf("failed to send nested media as document, %w", err)
				}
			}
		}
	}

	return nil
}

// sendAsDocument sends the media as a document to the chat
func (m *MediaSender) sendAsDocument(media *domain.Media) error {
	document := &telebot.Document{
		File:     telebot.FromURL(media.URL),
		FileName: fmt.Sprintf("%s.%s", media.ShortCode, getFileExtension(media.URL)),
	}

	_, err := m.bot.Send(m.msg.Chat, document)
	if err != nil {
		return fmt.Errorf("couldn't send the document, %w", err)
	}

	logging.Debugf("Sent media as document with short code [%v]", media.ShortCode)
	return nil
}

// sendCustomMessage sends a custom message as a reply
func (m *MediaSender) sendCustomMessage(message string) error {
	_, err := m.bot.Reply(m.msg, message)
	if err != nil {
		logging.Errorf("couldn't send custom message: %v", err)
		return err
	}
	return nil
}

// SendCaption will send the caption to the chat.
func (m *MediaSender) SendCaption(media *domain.Media) error {
	// If caption is empty, ignore sending it
	if media.Caption == "" {
		return nil
	}

	// Shrink media caption below 4096 characters
	if len(media.Caption) > 4096 {
		media.Caption = media.Caption[:4096]
	}

	_, err := m.bot.Reply(m.msg, media.Caption)
	return err
}

// getFileExtension extracts the file extension from a URL
func getFileExtension(url string) string {
	// A simple regex to get the file extension
	re := regexp.MustCompile(`\.(\w+)$`)
	match := re.FindStringSubmatch(url)
	if len(match) > 1 {
		return match[1]
	}
	return "file" // Default extension if we can't determine one
}
