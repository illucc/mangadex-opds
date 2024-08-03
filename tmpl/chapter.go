package tmpl

import (
	"io"

	"github.com/rushsteve1/mangadex-opds/models"
)

func ComicInfoXML(c *models.Chapter, w io.Writer) error {
	return tmpl.ExecuteTemplate(w, "comicinfo.tmpl.xml", c)
}

func ContentOPF(c *models.Chapter, w io.Writer) error {
	return tmpl.ExecuteTemplate(w, "content.tmpl.opf", c)
}

func TocNCX(c *models.Chapter, w io.Writer) error {
	return tmpl.ExecuteTemplate(w, "toc.tmpl.ncx", c)
}

func EpubXHTML(c *models.Chapter, w io.Writer) error {
	return tmpl.ExecuteTemplate(w, "epub.tmpl.xhtml", c)
}
