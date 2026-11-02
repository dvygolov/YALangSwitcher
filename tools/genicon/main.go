// Command genicon renders the application and tray icons into assets/.
//
//	go run ./tools/genicon
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/dvygolov/yalangswitcher/internal/icon"
)

func main() {
	out := flag.String("out", "assets", "output directory")
	preview := flag.Bool("preview", true, "also write PNG previews")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}

	if err := writeICO(filepath.Join(*out, "icon-en.ico"), "en"); err != nil {
		fail(err)
	}
	if err := writeICO(filepath.Join(*out, "icon-ru.ico"), "ru"); err != nil {
		fail(err)
	}
	if err := writeICO(filepath.Join(*out, "app.ico"), "app"); err != nil {
		fail(err)
	}

	if *preview {
		dir := filepath.Join(*out, "preview")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
		for _, size := range []int{16, 20, 24, 32, 48, 256} {
			savePNG(filepath.Join(dir, fmt.Sprintf("en-%d.png", size)), icon.RenderLang("en", size))
			savePNG(filepath.Join(dir, fmt.Sprintf("ru-%d.png", size)), icon.RenderLang("ru", size))
			savePNG(filepath.Join(dir, fmt.Sprintf("app-%d.png", size)), icon.RenderApp(size))
		}
	}
	fmt.Println("icons written to", *out)
}

func writeICO(path, kind string) error {
	imgs := make([]*image.RGBA, 0, len(icon.Sizes))
	for _, size := range icon.Sizes {
		switch kind {
		case "en":
			imgs = append(imgs, icon.RenderLang("en", size))
		case "ru":
			imgs = append(imgs, icon.RenderLang("ru", size))
		default:
			imgs = append(imgs, icon.RenderApp(size))
		}
	}
	if err := os.WriteFile(path, icon.EncodeICO(imgs), 0o644); err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("%-28s %6d bytes\n", path, fi.Size())
	return nil
}

func savePNG(path string, img *image.RGBA) {
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicon:", err)
	os.Exit(1)
}
