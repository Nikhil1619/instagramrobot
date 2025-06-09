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
	mediaList, err := GetGQLMediaList(repo.client, code)
	if err != nil {
		return domain.Media{}, fmt.Errorf("failed to get GraphQL media list: %w", err)
	}
	
	if len(mediaList) == 0 {
		return domain.Media{}, errors.New("no media found in GraphQL response")
	}
	
	// Return the first media item (GraphQL typically returns single media)
	return *mediaList[0], nil
}

// getPostWithEmbed uses the embed page parsing method
func (repo *Extractor) getPostWithEmbed(code string) (domain.Media, error) {
	// Try the new GraphQL embed parsing first
	mediaList, err := GetEmbedMediaList(repo.client, code)
	if err == nil && len(mediaList) > 0 {
		return *mediaList[0], nil
	}
	log.Printf("GraphQL embed method failed: %v", err)
	
	// Fall back to original embed parsing
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
	mediaList, err := GetIGramMediaList(repo.client, code)
	if err != nil {
		return domain.Media{}, fmt.Errorf("failed to get IGram media list: %w", err)
	}
	
	if len(mediaList) == 0 {
		return domain.Media{}, errors.New("no media items found")
	}

	// Handle single item vs multiple items
	if len(mediaList) == 1 {
		return *mediaList[0], nil
	}
	
	// For multiple items, create a media with Items array
	media := domain.Media{
		ShortCode: code,
		Source:    domain.SourceInstagram,
		Items:     make([]*domain.MediaItem, 0, len(mediaList)),
	}
	
	for _, m := range mediaList {
		item := &domain.MediaItem{
			URL:     m.URL,
			IsVideo: m.IsVideo,
		}
		media.Items = append(media.Items, item)
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
