package models

import (
	"context"
	"golang.org/x/time/rate"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/rushsteve1/mangadex-opds/shared"

	"github.com/google/uuid"
)

// Fetch gets the chapter information the MangaDex API and returns the [Chapter].
func FetchChapter(
	ctx context.Context,
	id uuid.UUID,
	queryParams url.Values,
) (c Chapter, err error) {
	slog.InfoContext(ctx, "fetching chapter", "id", id)

	queryPath, err := url.JoinPath("chapter", id.String())
	if err != nil {
		return c, err
	}

	if queryParams == nil {
		queryParams = url.Values{}
	}

	// Use reference expansion
	// https://api.mangadex.org/docs/01-concepts/reference-expansion/
	// TODO optimize these
	defaultParams := url.Values{
		"includes[]":           []string{"scanlation_group", "manga"},
		"translatedLanguage[]": []string{shared.GlobalOptions.Language},
	}

	for k, v := range defaultParams {
		queryParams[k] = v
	}

	data, err := shared.QueryAPI[Data[Chapter]](ctx, queryPath, queryParams, nil)

	data.Data.FullTitle()
	data.Data.Manga()
	_, err2 := data.Data.FetchImageURLs(ctx)
	if err2 != nil {
		err = err2
	}

	return data.Data, err
}

type imageUrlResponse struct {
	Result  string `json:"result"`
	BaseUrl string `json:"baseUrl"`
	Chapter struct {
		Hash      string   `json:"hash"`
		Data      []string `json:"data"`
		DataSaver []string `json:"dataSaver"`
	} `json:"chapter"`
}

var FetchImageURLsLimiter = rate.NewLimiter(rate.Every(time.Minute)/40, 40)

// FetchImageURLs gets the list of image URLs that correspond to this [Chapter].
// These URLs are not part of the normal MangaDex API, and are usually fetched
// from  the Mangadex@Home servers via mangadex.network.
// This function uses the DataSaver and MDUploads global options
//
// See also: https://api.mangadex.org/docs/04-chapter/retrieving-chapter/
func (c *Chapter) FetchImageURLs(ctx context.Context) (imgUrls []*url.URL, err error) {
	// Image urls are cached off in the chapter so that they don't need to be fetched multiple times
	if len(c.imgUrls) != 0 && c.Attributes.Pages != 0 {
		return c.imgUrls, nil
	}

	slog.InfoContext(ctx, "fetching image urls for chapter", "id", c.ID)

	// TODO support non MD-at-home
	queryPath, err := url.JoinPath("at-home", "server", c.ID.String())
	if err != nil {
		return nil, err
	}

	resp, err := shared.QueryAPI[imageUrlResponse](ctx, queryPath, nil, FetchImageURLsLimiter)
	if err != nil {
		return nil, err
	}

	var dName string
	var imgStrs []string
	if shared.GlobalOptions.DataSaver {
		dName = "data-saver"
		imgStrs = resp.Chapter.DataSaver
	} else {
		dName = "data"
		imgStrs = resp.Chapter.Data
	}

	// If this option is enabled then overwrire the base url
	if shared.GlobalOptions.MDUploads {
		resp.BaseUrl = shared.UploadsURL.String()
	}

	// Pre-allocate the slice
	imgUrls = make([]*url.URL, len(imgStrs), len(imgStrs))

	var wg sync.WaitGroup
	wg.Add(len(imgStrs))

	for i, imgStr := range imgStrs {
		go func(i int, imgStr string) {
			defer wg.Done()
			imgUrl, err2 := url.Parse(resp.BaseUrl)
			if err2 != nil {
				err = err2
			}

			imgUrl.Path, err2 = url.JoinPath(dName, resp.Chapter.Hash, imgStr)
			if err2 != nil {
				err = err2
			}

			imgUrls[i] = imgUrl
		}(i, imgStr)
	}

	wg.Wait()

	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "fetched image urls", "count", len(imgUrls))

	c.imgUrls = imgUrls

	return imgUrls, nil
}
