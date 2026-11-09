package chrome

import (
	"image"

	"github.com/ryoku-dev/ryotunes/internal/client/ui/look"
)

// Frame describes the shell geometry the top bar needs in window pixels.
type Frame struct {
	Class     look.Class
	Sidebar   int
	Content   image.Rectangle
	Decorated bool
	Maximized bool
}
