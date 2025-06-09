package instagram

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/gocolly/colly/v2"

	"github.com/omegaatt36/instagramrobot/domain"
)

// Extractor is the implement for fetching Instagram media.
type Extractor struct {
	client *http.Client
}

// NewInstagramFetcher will create a new instance of InstagramFetcherRepo.
func NewInstagramFetcher() domain.InstagramFetcher {
	return &Extractor{
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				Dial: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).Dial,
				TLSHandshakeTimeout: 5 * time.Second,
			},
		},
	}
}

// fromEmbedResponse will automatically transforms the EmbedResponse to the Media
func fromEmbedResponse(embed EmbedResponse) domain.Media {
	media := domain.Media{
		ShortCode: embed.Media.ShortCode,
		URL:       embed.ExtractMediaURL(),
		IsVideo:   embed.IsVideo(),
		Caption:   embed.GetCaption(),
	}

	for _, item := range embed.Media.SliderItems.Edges {
		media.Items = append(media.Items, &domain.MediaItem{
			IsVideo: item.Node.IsVideo,
			URL:     item.Node.ExtractMediaURL(),
		})
	}

	return media
}

// GetPostWithCode lets you to get information about specific Instagram post
// by providing its unique short code with multiple fallback methods
func (repo *Extractor) GetPostWithCode(code string) (domain.Media, error) {
	// Method 1: Try GraphQL API approach
	media, err := repo.getPostWithGQL(code)
	if err == nil {
		return media, nil
	}
	log.Printf("GQL method failed: %v", err)

	// Method 2: Try embed page parsing
	media, err = repo.getPostWithEmbed(code)
	if err == nil {
		return media, nil
	}
	log.Printf("Embed method failed: %v", err)

	// Method 3: Try third-party service
	media, err = repo.getPostWithThirdParty(code)
	if err == nil {
		return media, nil
	}
	log.Printf("Third-party method failed: %v", err)

	return domain.Media{}, errors.New("all extraction methods failed - the post might be private or the link is incorrect")
}

// getPostWithGQL attempts to fetch post data using GraphQL API
func (repo *Extractor) getPostWithGQL(code string) (domain.Media, error) {
	// This would require implementing GraphQL request similar to the reference
	// For now, return error to fall back to other methods
	return domain.Media{}, errors.New("GQL method not implemented yet")
}

// getPostWithEmbed uses the existing embed page parsing method
func (repo *Extractor) getPostWithEmbed(code string) (domain.Media, error) {
	URL := fmt.Sprintf("https://www.instagram.com/p/%v/embed/captioned/", code)

	var coverPhoto string
	var embedResponse = EmbedResponse{}
	collector := colly.NewCollector()
	collector.SetClient(repo.client)

	collector.OnHTML("img.EmbeddedMediaImage", func(e *colly.HTMLElement) {
		coverPhoto = e.Attr("src")
	})

	collector.OnHTML("script", func(e *colly.HTMLElement) {
		// Try multiple regex patterns for better compatibility
		patterns := []string{
			`\\\"gql_data\\\":([\s\S]*)\}\"\}\]\]\,\[\"NavigationMetrics`,
			`new ServerJS\(\)\);s\.handle\(({.*})\);requireLazy`,
			`window\._sharedData = ({.*?});`,
		}

		for _, pattern := range patterns {
			r := regexp.MustCompile(pattern)
			match := r.FindStringSubmatch(e.Text)

			if len(match) >= 2 {
				s := strings.ReplaceAll(match[1], `\"`, `"`)
				s = strings.ReplaceAll(s, `\\/`, `/`)
				s = strings.ReplaceAll(s, `\\`, `\`)

				if err := json.Unmarshal([]byte(s), &embedResponse); err == nil {
					return
				}
			}
		}
	})

	collector.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", browser.Random())
	})

	if err := collector.Visit(URL); err != nil {
		return domain.Media{}, fmt.Errorf("failed to send HTTP request: %w", err)
	}

	if !embedResponse.IsEmpty() {
		return fromEmbedResponse(embedResponse), nil
	}

	if coverPhoto != "" {
		return domain.Media{
			URL:     coverPhoto,
			Caption: "can only fetch the cover photo",
		}, nil
	}

	return domain.Media{}, errors.New("no data found in embed page")
}

// getPostWithThirdParty attempts to use a third-party service as fallback
func (repo *Extractor) getPostWithThirdParty(code string) (domain.Media, error) {
	postURL := fmt.Sprintf("https://www.instagram.com/p/%s/", code)
	
	apiURL := fmt.Sprintf("https://%s/api/convert", igramHostname)
	payload, err := BuildIGramPayload(postURL)
	if err != nil {
		return domain.Media{}, fmt.Errorf("failed to build signed payload: %w", err)
	}

	resp, err := FetchPage(repo.client, "POST", apiURL, payload, igramHeaders)
	if err != nil {
		return domain.Media{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return domain.Media{}, fmt.Errorf("failed to get response: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.Media{}, fmt.Errorf("failed to read response body: %w", err)
	}

	response, err := ParseIGramResponse(body)
	if err != nil {
		return domain.Media{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Items) == 0 {
		return domain.Media{}, errors.New("no media items found")
	}

	media := domain.Media{
		ShortCode: code,
		Source:    domain.SourceInstagram,
	}

	// Handle single item
	if len(response.Items) == 1 && len(response.Items[0].URL) > 0 {
		item := response.Items[0]
		urlObj := item.URL[0]
		
		contentURL, err := GetCDNURL(urlObj.URL)
		if err != nil {
			return domain.Media{}, fmt.Errorf("failed to get CDN URL: %w", err)
		}
		
		media.URL = contentURL
		media.IsVideo = urlObj.Ext == "mp4"
	} else {
		// Handle multiple items
		for _, item := range response.Items {
			if len(item.URL) > 0 {
				urlObj := item.URL[0]
				contentURL, err := GetCDNURL(urlObj.URL)
				if err != nil {
					continue // Skip this item if URL extraction fails
				}
				
				mediaItem := &domain.MediaItem{
					URL:     contentURL,
					IsVideo: urlObj.Ext == "mp4",
				}
				media.Items = append(media.Items, mediaItem)
			}
		}
	}

	return media, nil
}

// ExtractShortCodeFromLink will extract the media short code from a URL link or path
func ExtractShortCodeFromLink(link string) (string, error) {
	values := regexp.MustCompile(`(p|tv|reel|reels\/videos)\/([A-Za-z0-9-_]+)`).FindStringSubmatch(link)
	if len(values) != 3 {
		return "", errors.New("couldn't extract the media short code from the link")
	}
	// return short code
	return values[2], nil
}
