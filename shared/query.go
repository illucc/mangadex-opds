package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/allegro/bigcache/v3"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"runtime/debug"
	"time"
)

// APIUrl is the default MangaDex API URL
var APIUrl = url.URL{
	Scheme: "https",
	Host:   "api.mangadex.org",
}

// DevUrl is the MangaDex Dev API URL used in place of [APIUrl]
var DevUrl = url.URL{
	Scheme: "https",
	Host:   "api.mangadex.dev",
}

// UploadsURL is the MangaDex Uploads URL used when the MDUploads option is true
var UploadsURL = url.URL{
	Scheme: "https",
	Host:   "uploads.mangadex.org",
}

var cache, _ = bigcache.New(context.Background(), bigcache.Config{
	// number of shards (must be a power of 2)
	Shards: 1024,

	// time after which entry can be evicted
	LifeWindow: 10 * time.Minute,

	// Interval between removing expired entries (clean up).
	// If set to <= 0 then no action is performed.
	// Setting to < 1 second is counterproductive — bigcache has a one second resolution.
	CleanWindow: 5 * time.Minute,

	// rps * lifeWindow, used only in initial memory allocation
	MaxEntriesInWindow: 1000 * 10 * 60,

	// max entry size in bytes, used only in initial memory allocation
	MaxEntrySize: 500,

	// prints information about additional memory allocation
	Verbose: true,

	// cache will not allocate more memory than this limit, value in MB
	// if value is reached then the oldest entries can be overridden for the new ones
	// 0 value means no size limit
	HardMaxCacheSize: 450,

	// callback fired when the oldest entry is removed because of its expiration time or no space left
	// for the new entry, or because delete was called. A bitmask representing the reason will be returned.
	// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
	OnRemove: nil,

	// OnRemoveWithReason is a callback fired when the oldest entry is removed because of its expiration time or no space left
	// for the new entry, or because delete was called. A constant representing the reason will be passed through.
	// Default value is nil which means no callback and it prevents from unwrapping the oldest entry.
	// Ignored if OnRemove is specified.
	OnRemoveWithReason: nil,
})

// UserAgent constructs the `User-Agent` header from the build information.
func UserAgent() string {
	info, ok := debug.ReadBuildInfo()

	// I have no idea under what circumstances this is possible but
	// defensive programming is the way to go
	if !ok {
		slog.Error("could not read build info")
		panic("could not read build info")
	}

	return fmt.Sprintf("%s/%s", path.Base(info.Main.Path), info.Main.Version)
}

// QueryAPI is used to fetch data from the MangaDex API.
func QueryAPI[T any](
	ctx context.Context,
	queryPath string,
	queryParams url.Values,
) (out T, err error) {
	var queryUrl url.URL
	if GlobalOptions.DevApi {
		queryUrl = DevUrl
	} else {
		queryUrl = APIUrl
	}

	queryUrl.Path = queryPath
	queryUrl.RawQuery = queryParams.Encode()

	entry, err := cache.Get(queryUrl.String())
	if err != nil {
		slog.InfoContext(ctx, "querying API", "url", queryUrl.String())

		req, err := makeRequest(ctx, &queryUrl)
		if err != nil {
			return out, err
		}
		req.Header.Set("Accept", "application/json")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return out, err
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			return out, fmt.Errorf("upstream error: %s", res.Status)
		}

		entry, err = io.ReadAll(res.Body)
		if err != nil {
			return out, err
		}

		err = cache.Set(queryUrl.String(), entry)
		if err != nil {
			return out, err
		}
	} else {
		slog.InfoContext(ctx, "loading cache of API", "url", queryUrl.String())
	}
	err = json.NewDecoder(bytes.NewReader(entry)).Decode(&out)
	return out, err
}

// QueryImage is used to fetch an image from the given URL.
// Only PNG and JPG images are supported, for compatibility with downstream CBZ and EPUB formats.
func QueryImage(ctx context.Context, imgUrl *url.URL, w io.Writer) (err error) {
	// In some tests we do not actually want to download the files
	if GlobalOptions.NoDownload {
		slog.Warn("no-download option enabled", "url", imgUrl.String())
		return nil
	}

	slog.InfoContext(ctx, "querying image", "url", imgUrl.String())

	req, err := makeRequest(ctx, imgUrl)
	if err != nil {
		return err
	}

	req.Header.Add("Accept", mime.TypeByExtension(".png"))
	req.Header.Add("Accept", mime.TypeByExtension(".jpg"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream error: %s", res.Status)
	}

	_, err = io.Copy(w, res.Body)

	slog.DebugContext(ctx, "finished image download", "url", imgUrl)

	return err
}

func makeRequest(ctx context.Context, queryUrl *url.URL) (req *http.Request, err error) {
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, queryUrl.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent())

	return req, nil
}

// TODO make this more general so it can be used for Chapters
func WithDefaultParams(queryParams url.Values) url.Values {
	if queryParams == nil {
		queryParams = url.Values{}
	}

	// Use reference expansion
	// https://api.mangadex.org/docs/01-concepts/reference-expansion/
	// TODO optimize these
	defaultParams := url.Values{
		"includes[]": []string{"author", "artist", "cover_art"},
	}

	for k, v := range defaultParams {
		queryParams[k] = v
	}

	return queryParams
}
