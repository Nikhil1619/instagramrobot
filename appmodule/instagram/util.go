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
	
	// Navigate through the JSON structure to find contextJSON
	igCtx := traverseJSON(data, "contextJSON")
	if igCtx == nil {
		return nil, ErrGQLContextNotFound
	}
	
	var ctxJSON ContextJSON
	switch v := igCtx.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &ctxJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal contextJSON: %w", err)
		}
	default:
		return nil, ErrGQLContextMismatch
	}
	
	if ctxJSON.GqlData == nil {
		return nil, ErrGQLNilResponse
	}
	if ctxJSON.GqlData.ShortcodeMedia == nil {
		return nil, ErrGQLNilMedia
	}
	
	return ctxJSON.GqlData.ShortcodeMedia, nil
}

// traverseJSON navigates through nested JSON structure to find a key
func traverseJSON(data map[string]any, key string) any {
	if val, exists := data[key]; exists {
		return val
	}
	
	// Recursively search in nested objects
	for _, v := range data {
		if nested, ok := v.(map[string]any); ok {
			if result := traverseJSON(nested, key); result != nil {
				return result
			}
		}
	}
	
	return nil
}

// GetGQLData fetches Instagram post data using GraphQL API
func GetGQLData(client *http.Client, shortcode string) (*GraphQLData, error) {
	graphHeaders, body, err := BuildGQLData()
	if err != nil {
		return nil, fmt.Errorf("failed to build GQL data: %w", err)
	}
	
	formData := url.Values{}
	for key, value := range body {
		formData.Set(key, value)
	}
	formData.Set("fb_api_caller_class", "RelayModern")
	formData.Set("fb_api_req_friendly_name", polarisAction)
	
	variables := map[string]any{
		"shortcode":               shortcode,
		"fetch_tagged_user_count": nil,
		"hoisted_comment_id":      nil,
		"hoisted_reply_id":        nil,
	}
	
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal variables: %w", err)
	}
	
	formData.Set("variables", string(variablesJSON))
	formData.Set("server_timestamps", "true")
	formData.Set("doc_id", "8845758582119845")

	// Merge headers
	for key, value := range webHeaders {
		graphHeaders[key] = value
	}
	
	resp, err := FetchPage(client, "POST", graphQLEndpoint, strings.NewReader(formData.Encode()), graphHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid response code: %s", resp.Status)
	}
	
	var response GraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if response.Data == nil {
		return nil, ErrGQLNilResponse
	}
	if response.Status != "ok" {
		return nil, fmt.Errorf("status is not ok: %s", response.Status)
	}
	if response.Data.ShortcodeMedia == nil {
		return nil, ErrGQLNilMedia
	}
	
	return response.Data, nil
}

// BuildGQLData creates headers and body for GraphQL request
func BuildGQLData() (map[string]string, map[string]string, error) {
	const (
		domain                = "www"
		requestID             = "b"
		clientCapabilityGrade = "EXCELLENT"
		sessionInternalID     = "7436540909012459023"
		apiVersion            = "1"
		rolloutHash           = "1019933358"
		appID                 = "936619743392459"
		bloksVersionID        = "6309c8d03d8a3f47a1658ba38b304a3f837142ef5f637ebf1f8f52d4b802951e"
		asbdID                = "129477"
		hiddenState           = "20126.HYP:instagram_web_pkg.2.1...0"
		loggedIn              = "0"
		cometRequestID        = "7"
		appVersion            = "0"
		pixelRatio            = "2"
		buildType             = "trunk"
	)
	
	session := "::" + RandomAlphaString(6)
	sessionData := RandomBase64(8)
	csrfToken := RandomBase64(32)
	deviceID := RandomBase64(24)
	machineID := RandomBase64(24)
	dynamicFlags := RandomBase64(154)
	clientSessionRnd := RandomBase64(154)
	
	jazoestBig, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate jazoest: %w", err)
	}
	jazoest := strconv.FormatInt(jazoestBig.Int64()+1, 10)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	
	cookies := []string{
		"csrftoken=" + csrfToken,
		"ig_did=" + deviceID,
		"wd=1280x720",
		"dpr=2",
		"mid=" + machineID,
		"ig_nrcb=1",
	}
	
	headers := map[string]string{
		"x-ig-app-id":        appID,
		"X-FB-LSD":           sessionData,
		"X-CSRFToken":        csrfToken,
		"X-Bloks-Version-Id": bloksVersionID,
		"x-asbd-id":          asbdID,
		"cookie":             strings.Join(cookies, "; "),
		"Content-Type":       "application/x-www-form-urlencoded",
		"X-FB-Friendly-Name": polarisAction,
	}
	
	body := map[string]string{
		"__d":         domain,
		"__a":         apiVersion,
		"__s":         session,
		"__hs":        hiddenState,
		"__req":       requestID,
		"__ccg":       clientCapabilityGrade,
		"__rev":       rolloutHash,
		"__hsi":       sessionInternalID,
		"__dyn":       dynamicFlags,
		"__csr":       clientSessionRnd,
		"__user":      loggedIn,
		"__comet_req": cometRequestID,
		"libav":       appVersion,
		"dpr":         pixelRatio,
		"lsd":         sessionData,
		"jazoest":     jazoest,
		"__spin_r":    rolloutHash,
		"__spin_b":    buildType,
		"__spin_t":    timestamp,
	}
	
	return headers, body, nil
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

// GetGQLMediaList fetches Instagram media using GraphQL API
func GetGQLMediaList(client *http.Client, shortcode string) ([]*domain.Media, error) {
	gqlData, err := GetGQLData(client, shortcode)
	if err != nil {
		return nil, fmt.Errorf("failed to get GraphQL data: %w", err)
	}
	
	return ParseGQLMedia(gqlData.ShortcodeMedia, shortcode)
}

// GetEmbedMediaList fetches Instagram media using embed page parsing
func GetEmbedMediaList(client *http.Client, shortcode string) ([]*domain.Media, error) {
	embedURL := fmt.Sprintf("https://www.instagram.com/p/%s/embed/captioned/", shortcode)
	
	resp, err := FetchPage(client, "GET", embedURL, nil, webHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch embed page: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid response code: %s", resp.Status)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	media, err := ParseEmbedGQL(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embed GQL: %w", err)
	}
	
	return ParseGQLMedia(media, shortcode)
}

// GetIGramMediaList fetches Instagram media using third-party service
func GetIGramMediaList(client *http.Client, shortcode string) ([]*domain.Media, error) {
	contentURL := fmt.Sprintf("https://www.instagram.com/p/%s/", shortcode)
	
	payload, err := BuildIGramPayload(contentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to build IGram payload: %w", err)
	}
	
	igramURL := fmt.Sprintf("https://%s/api/convert", igramHostname)
	resp, err := FetchPage(client, "POST", igramURL, payload, igramHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to send IGram request: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid IGram response code: %s", resp.Status)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read IGram response body: %w", err)
	}
	
	igramResponse, err := ParseIGramResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse IGram response: %w", err)
	}
	
	mediaList := make([]*domain.Media, 0, len(igramResponse.Items))
	
	for _, item := range igramResponse.Items {
		if len(item.URL) == 0 {
			continue
		}
		
		urlObj := item.URL[0]
		cdnURL, err := GetCDNURL(urlObj.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to get CDN URL: %w", err)
		}
		
		media := &domain.Media{
			ShortCode: shortcode,
			URL:       cdnURL,
			IsVideo:   urlObj.Ext == "mp4",
			Caption:   item.Title,
			Source:    domain.SourceInstagram,
		}
		mediaList = append(mediaList, media)
	}
	
	return mediaList, nil
}