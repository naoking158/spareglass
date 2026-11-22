//go:build !(darwin && cgo)

package media

// CursorPosition is nil here: these pipelines draw the pointer into the video.
var CursorPosition CursorFunc
