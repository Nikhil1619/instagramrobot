package instagram

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/omegaatt36/instagramrobot/domain"
)

// GetGQLMediaList fetches media using GraphQL API and converts to domain media
func GetGQLMediaList(client *http.Client, shortcode string) ([]*domain.Media, error) {
	graphData, err := GetGQLData(client, shortcode)
	if err != nil {
		return nil, fmt.Errorf("failed to get graph data: %w", err)
	}
	return ParseGQLMedia(graphData.ShortcodeMedia, shortcode)
}

// GetEmbedMediaList fetches media from embed page and parses it
func GetEmbedMediaList(client *http.Client, shortcode string) ([]*domain.Media, error) {
	embedURL := fmt.Sprintf("https://www.instagram.com/p/%s/embed/captioned", shortcode)
	
	resp, err := FetchPage(client, http.MethodGet, embedURL, nil, webHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get embed page: %s", resp.Status)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	graphData, err := ParseEmbedGQL(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embed page: %w", err)
	}
	
	return ParseGQLMedia(graphData, shortcode)
}

// GetIGramMediaList fetches media using third-party service
func GetIGramMediaList(client *http.Client, shortcode string) ([]*domain.Media, error) {
	postURL := fmt.Sprintf("https://www.instagram.com/p/%s/", shortcode)
	details, err := GetFromIGram(client, postURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get post: %w", err)
	}
	
	mediaList := make([]*domain.Media, 0, len(details.Items))
	for _, item := range details.Items {
		if len(item.URL) == 0 {
			continue
		}
		
		urlObj := item.URL[0]
		contentURL, err := GetCDNURL(urlObj.URL)
		if err != nil {
			return nil, err
		}
		
		media := &domain.Media{
			ShortCode: shortcode,
			URL:       contentURL,
			IsVideo:   urlObj.Ext == "mp4",
			Source:    domain.SourceInstagram,
		}
		
		mediaList = append(mediaList, media)
	}

	return mediaList, nil
}

// GetFromIGram makes a request to the third-party IGram service
func GetFromIGram(client *http.Client, contentURL string) (*IGramResponse, error) {
	apiURL := fmt.Sprintf("https://%s/api/convert", igramHostname)
	payload, err := BuildIGramPayload(contentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build signed payload: %w", err)
	}
	
	resp, err := FetchPage(client, http.MethodPost, apiURL, payload, igramHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get response: %s", resp.Status)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	response, err := ParseIGramResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	return response, nil
}