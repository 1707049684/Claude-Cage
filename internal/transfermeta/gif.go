package transfermeta

import (
	"io"
	"strings"
)

// GIF comment extensions and XMP application extensions carry free-form
// provenance data. Graphic control, plain-text and other extensions are kept
// because they affect playback or the rendered image.
func sanitizeGIF(src io.ReadSeeker, dst io.Writer) (bool, error) {
	header := make([]byte, 6)
	if _, err := io.ReadFull(src, header); err != nil {
		return false, nil
	}
	if string(header) != "GIF87a" && string(header) != "GIF89a" {
		return false, nil
	}
	if _, err := dst.Write(header); err != nil {
		return false, err
	}
	screen := make([]byte, 7)
	if _, err := io.ReadFull(src, screen); err != nil {
		return false, nil
	}
	if _, err := dst.Write(screen); err != nil {
		return false, err
	}
	if screen[4]&0x80 != 0 {
		tableSize := 3 << ((screen[4] & 0x07) + 1)
		table := make([]byte, tableSize)
		if _, err := io.ReadFull(src, table); err != nil {
			return false, nil
		}
		if _, err := dst.Write(table); err != nil {
			return false, err
		}
	}
	changed := false
	for {
		var introducer [1]byte
		if _, err := io.ReadFull(src, introducer[:]); err != nil {
			return false, nil
		}
		switch introducer[0] {
		case 0x3B: // trailer
			if _, err := dst.Write(introducer[:]); err != nil {
				return false, err
			}
			if _, err := io.Copy(dst, src); err != nil {
				return false, err
			}
			return changed, nil
		case 0x2C: // image descriptor
			if _, err := dst.Write(introducer[:]); err != nil {
				return false, err
			}
			descriptor := make([]byte, 9)
			if _, err := io.ReadFull(src, descriptor); err != nil {
				return false, nil
			}
			if _, err := dst.Write(descriptor); err != nil {
				return false, err
			}
			if descriptor[8]&0x80 != 0 {
				tableSize := 3 << ((descriptor[8] & 0x07) + 1)
				table := make([]byte, tableSize)
				if _, err := io.ReadFull(src, table); err != nil {
					return false, nil
				}
				if _, err := dst.Write(table); err != nil {
					return false, err
				}
			}
			var codeSize [1]byte
			if _, err := io.ReadFull(src, codeSize[:]); err != nil {
				return false, nil
			}
			if _, err := dst.Write(codeSize[:]); err != nil {
				return false, err
			}
			if err := copyGIFSubBlocks(src, dst, true); err != nil {
				return false, err
			}
		case 0x21: // extension introducer
			var label [1]byte
			if _, err := io.ReadFull(src, label[:]); err != nil {
				return false, nil
			}
			if label[0] == 0xFE { // comment extension
				if err := copyGIFSubBlocks(src, io.Discard, false); err != nil {
					return false, err
				}
				changed = true
				continue
			}
			if label[0] == 0xFF { // application extension
				identifier := make([]byte, 11)
				if _, err := io.ReadFull(src, identifier); err != nil {
					return false, nil
				}
				if strings.Contains(strings.ToUpper(string(identifier)), "XMP") {
					if err := copyGIFSubBlocks(src, io.Discard, false); err != nil {
						return false, err
					}
					changed = true
					continue
				}
				if _, err := dst.Write([]byte{0x21, 0xFF}); err != nil {
					return false, err
				}
				if _, err := dst.Write(identifier); err != nil {
					return false, err
				}
				if err := copyGIFSubBlocks(src, dst, true); err != nil {
					return false, err
				}
				continue
			}
			if _, err := dst.Write([]byte{0x21, label[0]}); err != nil {
				return false, err
			}
			if err := copyGIFSubBlocks(src, dst, true); err != nil {
				return false, err
			}
		default:
			return false, nil
		}
	}
}

func copyGIFSubBlocks(src io.Reader, dst io.Writer, keep bool) error {
	for {
		var size [1]byte
		if _, err := io.ReadFull(src, size[:]); err != nil {
			return err
		}
		if size[0] == 0 {
			if keep {
				_, err := dst.Write(size[:])
				return err
			}
			return nil
		}
		if keep {
			if _, err := dst.Write(size[:]); err != nil {
				return err
			}
			if _, err := io.CopyN(dst, src, int64(size[0])); err != nil {
				return err
			}
			continue
		}
		if _, err := io.CopyN(io.Discard, src, int64(size[0])); err != nil {
			return err
		}
	}
}
