package instagram

import (
	"fmt"
	"io"
	"net/http"

	"github.com/omegaatt36/instagramrobot/domain"
)

// GraphQL data structures for Instagram API responses

// GraphQLResponse represents the main GraphQL response
type GraphQLResponse struct {
	Data   *GraphQLData `json:"data"`
	Status string       `json:"status"`
}

// GraphQLData contains the actual media data
type GraphQLData struct {
	ShortcodeMedia *Media `json:"shortcode_media"`
}

// Media represents Instagram media data
type Media struct {
	ID                    string                `json:"id"`
	Shortcode             string                `json:"shortcode"`
	Typename              string                `json:"__typename"`
	DisplayURL            string                `json:"display_url"`
	VideoURL              string                `json:"video_url"`
	IsVideo               bool                  `json:"is_video"`
	EdgeMediaToCaption    *EdgeMediaToCaption   `json:"edge_media_to_caption"`
	EdgeSidecarToChildren *EdgeSidecarToChildren `json:"edge_sidecar_to_children"`
}

// EdgeMediaToCaption contains caption data
type EdgeMediaToCaption struct {
	Edges []CaptionEdge `json:"edges"`
}

// CaptionEdge represents a caption edge
type CaptionEdge struct {
	Node CaptionNode `json:"node"`
}

// CaptionNode contains the actual caption text
type CaptionNode struct {
	Text string `json:"text"`
}

// EdgeSidecarToChildren contains carousel/sidecar media
type EdgeSidecarToChildren struct {
	Edges []SidecarEdge `json:"edges"`
}

// SidecarEdge represents a sidecar edge
type SidecarEdge struct {
	Node SidecarNode `json:"node"`
}

// SidecarNode contains individual media in carousel
type SidecarNode struct {
	ID         string `json:"id"`
	Typename   string `json:"__typename"`
	DisplayURL string `json:"display_url"`
	VideoURL   string `json:"video_url"`
	IsVideo    bool   `json:"is_video"`
}

// ContextJSON represents the context JSON from embed pages
type ContextJSON struct {
	GqlData *GraphQLData `json:"gql_data"`
}

// IGramResponse represents response from third-party service
type IGramResponse struct {
	Items []*IGramMedia `json:"items"`
}

// IGramMedia represents media item from third-party service
type IGramMedia struct {
	URL   []IGramURL `json:"url"`
	Type  string     `json:"type"`
	Title string     `json:"title"`
}

// IGramURL represents URL data from IGram service
type IGramURL struct {
	URL string `json:"url"`
	Ext string `json:"ext"`
}

// Error definitions for GraphQL operations
var (
	ErrGQLJSONNotFound     = fmt.Errorf("GraphQL JSON not found in response")
	ErrGQLContextNotFound  = fmt.Errorf("GraphQL context not found")
	ErrGQLContextMismatch  = fmt.Errorf("GraphQL context type mismatch")
	ErrGQLNilResponse      = fmt.Errorf("GraphQL response data is nil")
	ErrGQLNilMedia         = fmt.Errorf("GraphQL media data is nil")
)