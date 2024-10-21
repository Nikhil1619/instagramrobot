package telegram

import (
	"fmt"
	"regexp"

	"gopkg.in/telebot.v3"

	"github.com/omegaatt36/instagramrobot/domain"
	"github.com/omegaatt36/instagramrobot/logging"
)

type MediaSender struct {
	bot *telebot.Bot
	msg *telebot.Message
}

// NewMediaSender creates a new MediaSenderImpl instance
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

	if err := fnSend(media); err != nil {
		return fmt.Errorf("sending the media failed, %w", err)
	}

	return m.SendCaption(media)
}

func (m *MediaSender) sendSingleMedia(media *domain.Media) error {
	if media.URL == "" {
		return nil
	}

	mediaToSend := convertMediaToInputtable(media)

	// Send the media normally (e.g., as photo or video)
	if _, err := m.bot.Send(m.msg.Chat, mediaToSend); err != nil {
		return fmt.Errorf("couldn't send the %s media, %w", mediaToSend.MediaType(), err)
	}

	logging.Debugf("Sent single %s with short code [%v]", mediaToSend.MediaType(), media.ShortCode)

	// Now, send the media as a document
	if err := m.sendAsDocument(media); err != nil {
		return fmt.Errorf("failed to send as document, %w", err)
	}

	return nil
}

// sendNestedMedia will handle the case where there are more than 10 media items by splitting them into batches of 10 or fewer
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

		// Send the album for the current batch
		_, err := m.bot.SendAlbum(m.msg.Chat, album)
		if err != nil {
			return fmt.Errorf("couldn't send the nested media, %w", err)
		}

		// Now, send each media item in the batch as a document
		for _, mediaItem := range media.Items[i:end] {
			if err := m.sendAsDocument(&domain.Media{
				URL:      mediaItem.URL,
				ShortCode: media.ShortCode, // Optional: Use same shortcode or generate new
			}); err != nil {
				return fmt.Errorf("failed to send nested media as document, %w", err)
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

// SendCaption will send the caption to the chat.
func (m *MediaSender) SendCaption(media *domain.Media) error {
	// If caption is empty, ignore sending it
	if media.Caption == "" {
		return nil
	}

	// shrink media caption below 4096 characters
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
