package notionapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	// TODO: add more values, see FormatPage struct
	validFormatValues = map[string]struct{}{
		"page_full_width": struct{}{},
		"page_small_text": struct{}{},
	}
)

// Page describes a single Notion page
type Page struct {
	ID string
	// Root is a root block representing a page
	Root *Block
	// Users allows to find users that Page refers to by their ID
	Users  []*User
	Tables []*Table

	client *Client
}

// Table represents a table (i.e. CollectionView)
type Table struct {
	CollectionView *CollectionView `json:"collection_view"`
	Collection     *Collection     `json:"collection"`
	Data           []*Block
}

// SetTitle changes page title
func (p *Page) SetTitle(s string) error {
	op := buildSetTitleOp(p.Root.ID, s)
	ops := []*Operation{op}
	return p.client.SubmitTransaction(ops)
}

// SetFormat changes format properties of a page. Valid values are:
// page_full_width (bool), page_small_text (bool)
func (p *Page) SetFormat(args map[string]interface{}) error {
	if len(args) == 0 {
		return errors.New("args can't be empty")
	}
	for k := range args {
		if _, ok := validFormatValues[k]; !ok {
			return fmt.Errorf("'%s' is not a valid page format property", k)
		}
	}
	op := buildSetPageFormat(p.Root.ID, args)
	ops := []*Operation{op}
	return p.client.SubmitTransaction(ops)
}

func getFirstInline(inline []*InlineBlock) string {
	if len(inline) == 0 {
		return ""
	}
	return inline[0].Text
}

func getFirstInlineBlock(v interface{}) (string, error) {
	inline, err := parseInlineBlocks(v)
	if err != nil {
		return "", err
	}
	return getFirstInline(inline), nil
}

func getProp(block *Block, name string, toSet *string) bool {
	v, ok := block.Properties[name]
	if !ok {
		return false
	}
	s, err := getFirstInlineBlock(v)
	if err != nil {
		return false
	}
	*toSet = s
	return true
}

func parseProperties(block *Block) error {
	var err error
	props := block.Properties

	if title, ok := props["title"]; ok {
		if block.Type == BlockPage {
			block.Title, err = getFirstInlineBlock(title)
		} else if block.Type == BlockCode {
			block.Code, err = getFirstInlineBlock(title)
		} else {
			block.InlineContent, err = parseInlineBlocks(title)
		}
		if err != nil {
			return err
		}
	}

	if BlockTodo == block.Type {
		if checked, ok := props["checked"]; ok {
			s, _ := getFirstInlineBlock(checked)
			// fmt.Printf("checked: '%s'\n", s)
			block.IsChecked = strings.EqualFold(s, "Yes")
		}
	}

	// for BlockBookmark
	getProp(block, "description", &block.Description)
	// for BlockBookmark
	getProp(block, "link", &block.Link)

	// for BlockBookmark, BlockImage, BlockGist, BlockFile, BlockEmbed
	// don't over-write if was already set from "source" json field
	if block.Source == "" {
		getProp(block, "source", &block.Source)
	}

	if block.Source != "" && block.IsImage() {
		block.ImageURL = makeImageURL(block.Source)
	}

	// for BlockCode
	getProp(block, "language", &block.CodeLanguage)

	// for BlockFile
	if block.Type == BlockFile {
		getProp(block, "size", &block.FileSize)
	}

	return nil
}

// sometimes image url in "source" is not accessible but can
// be accessed when proxied via notion server as
// www.notion.so/image/${source}
// This also allows resizing via ?width=${n} arguments
//
// from: /images/page-cover/met_vincent_van_gogh_cradle.jpg
// =>
// https://www.notion.so/image/https%3A%2F%2Fwww.notion.so%2Fimages%2Fpage-cover%2Fmet_vincent_van_gogh_cradle.jpg?width=3290
func makeImageURL(uri string) string {
	if uri == "" || strings.Contains(uri, "//www.notion.so/image/") {
		return uri
	}
	// if the url has https://, it's already in s3.
	// If not, it's only a relative URL (like those for built-in
	// cover pages)
	if !strings.HasPrefix(uri, "https://") {
		uri = "https://www.notion.so" + uri
	}
	return "https://www.notion.so/image/" + url.PathEscape(uri)
}

func parseFormat(block *Block) error {
	if len(block.FormatRaw) == 0 {
		// TODO: maybe if BlockPage, set to default &FormatPage{}
		return nil
	}
	var err error
	switch block.Type {
	case BlockPage:
		var format FormatPage
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			format.PageCoverURL = makeImageURL(format.PageCover)
			block.FormatPage = &format
		}
	case BlockBookmark:
		var format FormatBookmark
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			block.FormatBookmark = &format
		}
	case BlockImage:
		var format FormatImage
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			format.ImageURL = makeImageURL(format.DisplaySource)
			block.FormatImage = &format
		}
	case BlockColumn:
		var format FormatColumn
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			block.FormatColumn = &format
		}
	case BlockTable:
		var format FormatTable
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			block.FormatTable = &format
		}
	case BlockText:
		var format FormatText
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			block.FormatText = &format
		}
	case BlockVideo:
		var format FormatVideo
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			block.FormatVideo = &format
		}
	case BlockEmbed:
		var format FormatEmbed
		err = json.Unmarshal(block.FormatRaw, &format)
		if err == nil {
			block.FormatEmbed = &format
		}
	}

	if err != nil {
		fmt.Printf("parseFormat: json.Unamrshal() failed with '%s', format: '%s'\n", err, string(block.FormatRaw))
		return err
	}
	return nil
}
