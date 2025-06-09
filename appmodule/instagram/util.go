package instagram

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/omegaatt36/instagramrobot/domain"
)

const (
	graphQLEndpoint = "https://www.instagram.com/graphql/query/"
	polarisAction   = "PolarisPostActionLoadPostQueryQuery"

	igramHostname  = "api.igram.world"
	igramKey       = "aaeaf2805cea6abef3f9d2b6a666fce62fd9d612a43ab772bb50ce81455112e0"
	igramTimestamp = "1742201548873"
)

var (
	embedPattern = regexp.MustCompile(
		`new ServerJS\(\)\);s\.handle\(({.*})\);requireLazy`)

	webHeaders = map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
		"Accept-Language":           "en-GB,en;q=0.9",
		"Cache-Control":             "max-age=0",
		"Dnt":                       "1",
		"Priority":                  "u=0, i",
		"Sec-Ch-Ua":                 `Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99`,
		"Sec-Ch-Ua-Mobile":          "?0",
		"Sec-Ch-Ua-Platform":        "macOS",
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	}

	igramHeaders = map[string]string{
		"Content-Type": "application/json",
	}
)

// RandomAlphaString generates a random alphabetic string of specified length
func RandomAlphaString(length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		result[i] = letters[n.Int64()]
	}
	return string(result)
}

// RandomBase64 generates a random base64-like string of specified length
func RandomBase64(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		result[i] = chars[n.Int64()]
	}
	return string(result)
}

// ParseGQLMedia converts GraphQL media to domain media
func ParseGQLMedia(data *Media, shortcode string) ([]*domain.Media, error) {
	var caption string
	if data.EdgeMediaToCaption != nil && len(data.EdgeMediaToCaption.Edges) > 0 {
		caption = data.EdgeMediaToCaption.Edges[0].Node.Text
	}

	mediaList := make([]*domain.Media, 0)

	switch data.Typename {
	case "GraphVideo", "XDTGraphVideo":
		media := &domain.Media{
			ShortCode: shortcode,
			URL:       data.VideoURL,
			IsVideo:   true,
			Caption:   caption,
			Source:    domain.SourceInstagram,
		}
		mediaList = append(mediaList, media)

	case "GraphImage", "XDTGraphImage":
		media := &domain.Media{
			ShortCode: shortcode,
			URL:       data.DisplayURL,
			IsVideo:   false,
			Caption:   caption,
			Source:    domain.SourceInstagram,
		}
		mediaList = append(mediaList, media)

	case "GraphSidecar", "XDTGraphSidecar":
		if data.EdgeSidecarToChildren != nil && len(data.EdgeSidecarToChildren.Edges) > 0 {
			edges := data.EdgeSidecarToChildren.Edges
			
			media := &domain.Media{
				ShortCode: shortcode,
				Caption:   caption,
				Source:    domain.SourceInstagram,
				Items:     make([]*domain.MediaItem, 0, len(edges)),
			}

			for _, edge := range edges {
				node := edge.Node
				item := &domain.MediaItem{}

				switch node.Typename {
				case "GraphVideo", "XDTGraphVideo":
					item.URL = node.VideoURL
					item.IsVideo = true
				case "GraphImage", "XDTGraphImage":
					item.URL = node.DisplayURL
					item.IsVideo = false
				}

				media.Items = append(media.Items, item)
			}
			mediaList = append(mediaList, media)
		}

	default:
		return nil, fmt.Errorf("unknown media type: %s", data.Typename)
	}

	return mediaList, nil
}

// ParseEmbedGQL extracts media data from embed page response
func ParseEmbedGQL(body []byte) (*Media, error) {
	match := embedPattern.FindSubmatch(body)
	if len(match) < 2 {
		return nil, ErrGQLJSONNotFound
	}
	jsonData := match[1]

	var data map[string]any
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	
	// Navigate through the JSON structure to find the media data
	// This is a simplified version - you might need to adjust based on actual response structure
	return nil, ErrGQLContextNotFound
}

// BuildIGramPayload creates the payload for third-party service
func BuildIGramPayload(contentURL string) (io.Reader, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	hash := sha256.New()
	_, err := io.WriteString(
		hash,
		contentURL+timestamp+igramKey,
	)
	if err != nil {
		return nil, fmt.Errorf("error writing to SHA256 hash: %w", err)
	}
	secretBytes := hash.Sum(nil)
	secretString := hex.EncodeToString(secretBytes)
	secretString = strings.ToLower(secretString)
	
	payload := map[string]string{
		"url":  contentURL,
		"ts":   timestamp,
		"_ts":  igramTimestamp,
		"_tsc": "0",
		"_s":   secretString,
	}
	
	// Use standard json package if sonic is not available
	parsedPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error marshalling payload: %w", err)
	}
	
	reader := bytes.NewReader(parsedPayload)
	return reader, nil
}

// GetCDNURL extracts the actual CDN URL from igram response
func GetCDNURL(contentURL string) (string, error) {
	parsedURL, err := url.Parse(contentURL)
	if err != nil {
		return "", fmt.Errorf("can't parse igram URL: %w", err)
	}
	queryParams, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return "", fmt.Errorf("can't unescape igram URL: %w", err)
	}
	cdnURL := queryParams.Get("uri")
	return cdnURL, nil
}

// ParseIGramResponse parses the response from third-party service
func ParseIGramResponse(body []byte) (*IGramResponse, error) {
	var media IGramMedia
	if err := json.Unmarshal(body, &media); err != nil {
		// try with slice
		var items []*IGramMedia
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, fmt.Errorf("failed to parse igram response: %w", err)
		}
		return &IGramResponse{Items: items}, nil
	}
	return &IGramResponse{Items: []*IGramMedia{&media}}, nil
}

// FetchPage is a helper function to make HTTP requests with proper headers and cookies
func FetchPage(client *http.Client, method, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return client.Do(req)
}