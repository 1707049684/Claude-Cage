package transfermeta

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

var fixedZipTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func sanitizeZipDoc(name string, src *os.File, dst io.Writer, size int64, kind format) (bool, error) {
	reader, err := zip.NewReader(src, size)
	if err != nil {
		return false, nil
	}
	return sanitizeZipArchive(reader, dst, kind)
}

func sanitizeZipBytes(name string, data []byte, dst io.Writer, kind format) (bool, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false, nil
	}
	return sanitizeZipArchive(reader, dst, kind)
}

func sanitizeZipArchive(reader *zip.Reader, dst io.Writer, kind format) (bool, error) {
	writer := zip.NewWriter(dst)
	changed := false
	for _, entry := range reader.File {
		data, err := readZipEntry(entry)
		if err != nil {
			return false, err
		}
		content := data
		switch {
		case kind == formatOOXML && ooxmlMetadataName(entry.Name):
			if local := ooxmlMetadataRoot(entry.Name); local != "" {
				if cleaned, ok := clearXMLRoot(data, local); ok {
					content = cleaned
					changed = true
				}
			}
		case kind == formatODF && entry.Name == "meta.xml":
			if cleaned, ok := clearODFMeta(data); ok {
				content = cleaned
				changed = true
			}
		case kind == formatODF && entry.Name == "settings.xml":
			if cleaned, ok := removeODFLanguage(data); ok {
				content = cleaned
				changed = true
			}
		}
		if imageEntryName(entry.Name) {
			if cleaned, err := SanitizeBytes(entry.Name, content); err == nil && !bytes.Equal(cleaned, content) {
				content = cleaned
				changed = true
			}
		}
		header := entry.FileHeader
		header.Modified = fixedZipTime
		header.Extra = nil
		header.CRC32 = 0
		header.CompressedSize = 0
		header.UncompressedSize = 0
		header.CompressedSize64 = 0
		header.UncompressedSize64 = 0
		part, err := writer.CreateHeader(&header)
		if err != nil {
			return false, err
		}
		if _, err := part.Write(content); err != nil {
			return false, err
		}
	}
	if err := writer.Close(); err != nil {
		return false, err
	}
	return changed, nil
}

func readZipEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func ooxmlMetadataName(name string) bool {
	switch name {
	case "docProps/core.xml", "docProps/app.xml", "docProps/custom.xml":
		return true
	}
	return false
}

func ooxmlMetadataRoot(name string) string {
	switch name {
	case "docProps/core.xml":
		return "coreProperties"
	case "docProps/app.xml", "docProps/custom.xml":
		return "Properties"
	}
	return ""
}

func imageEntryName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}

var xmlPrefix = `(?:[\w.-]+:)?`

func clearXMLRoot(data []byte, localName string) ([]byte, bool) {
	if localName == "" {
		return data, false
	}
	expression := regexp.MustCompile(`(?s)(<(` + xmlPrefix + regexp.QuoteMeta(localName) + `)\b[^>]*>).*?</` + xmlPrefix + regexp.QuoteMeta(localName) + `>`)
	match := expression.FindSubmatch(data)
	if match == nil {
		return data, false
	}
	out := append([]byte{}, match[1]...)
	out = append(out, []byte("</"+string(match[2])+">")...)
	return out, true
}

func clearODFMeta(data []byte) ([]byte, bool) {
	expression := regexp.MustCompile(`(?s)(<(` + xmlPrefix + `meta)\b[^>]*>).*?</` + xmlPrefix + `meta>`)
	match := expression.FindSubmatch(data)
	if match == nil {
		return data, false
	}
	open := strings.TrimSuffix(string(match[1]), ">") + "/>"
	return expression.ReplaceAll(data, []byte(open)), true
}

func removeODFLanguage(data []byte) ([]byte, bool) {
	paired := regexp.MustCompile(`(?s)<config:config-item\b[^>]*config:name="(?:Language|Locale|TimeZone)"[^>]*>.*?</config:config-item>`)
	selfClosing := regexp.MustCompile(`<config:config-item\b[^>]*config:name="(?:Language|Locale|TimeZone)"[^>]*/>`)
	if paired.Match(data) {
		return paired.ReplaceAll(data, nil), true
	}
	if selfClosing.Match(data) {
		return selfClosing.ReplaceAll(data, nil), true
	}
	return data, false
}
