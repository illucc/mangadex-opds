package formats

import (
	"archive/zip"
	"context"
	"fmt"
	"github.com/rushsteve1/mangadex-opds/shared"
	"github.com/rushsteve1/mangadex-opds/tmpl"
	"golang.org/x/sync/errgroup"
	"io"
	"log/slog"
	"mime"
	"path"

	"github.com/rushsteve1/mangadex-opds/models"
)

// WriteEpub will write an EPUB file for the current [Chapter] to the given [io.Writer].
func WriteEpub(ctx context.Context, c *models.Chapter, w io.Writer) (err error) {
	z := zip.NewWriter(w)

	err = z.SetComment(c.FullTitle())
	if err != nil {
		return err
	}

	w, err = z.Create("mimetype")
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, mime.TypeByExtension(".epub"))
	if err != nil {
		return err
	}

	w, err = z.Create("META-INF/container.xml")
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
   </rootfiles>
</container>`)
	if err != nil {
		return err
	}

	w, err = z.Create("ComicInfo.xml")
	if err != nil {
		return err
	}

	err = tmpl.ComicInfoXML(c, w)
	if err != nil {
		return err
	}

	imgUrls, err := c.FetchImageURLs(ctx)
	if err != nil {
		return err
	}

	w, err = z.Create("OEBPS/content.opf")
	if err != nil {
		return err
	}

	err = tmpl.ContentOPF(c, w)
	if err != nil {
		return err
	}

	w, err = z.Create("OEBPS/toc.ncx")
	if err != nil {
		return err
	}

	err = tmpl.TocNCX(c, w)
	if err != nil {
		return err
	}

	w, err = z.Create("OEBPS/Text/epub.xhtml")
	if err != nil {
		return err
	}

	err = tmpl.EpubXHTML(c, w)
	if err != nil {
		return err
	}

	imgChan := make(chan chapterImage)
	doneChan := make(chan error)

	// Fetch and add the image files in parallel
	go func() {
		eg, ctx := errgroup.WithContext(ctx)
		eg.SetLimit(3)

		for _, img := range imgUrls {
			eg.Go(func() error {
				imgName := path.Base(img.String())
				chImg := chapterImage{Name: imgName}

				err := shared.QueryImage(ctx, img, &chImg.Data)
				if err != nil {
					return err
				}

				imgChan <- chImg

				return nil
			})
		}

		// Wait for all downloads to finish
		err = eg.Wait()
		close(imgChan)
		doneChan <- err

		slog.InfoContext(ctx, "done downloading images", "count", len(imgUrls))
	}()

	for img := range imgChan {
		// Images will not be compressed, just stored
		// This saves a lot of time and performance at the cost of bigger files
		// But considering MangaDex is fine with hosting those I assume they're already optimized
		w, err = z.CreateHeader(&zip.FileHeader{
			Name:   fmt.Sprintf("OEBPS/Images/%s", img.Name),
			Method: zip.Store,
		})
		if err != nil {
			return err
		}

		_, err = io.Copy(w, &img.Data)
		if err != nil {
			return err
		}
	}

	err = <-doneChan
	if err != nil {
		return err
	}

	err = z.Close()
	if err != nil {
		return err
	}

	return nil
}
