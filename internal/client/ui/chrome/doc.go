// Package chrome draws the persistent music-player frame around retained pages.
//
// The shell owns pane planes and window lifetime. Chrome owns the top bar,
// navigation, player surfaces, side panels, overlays opened by those surfaces,
// and their retained interaction state. The player surfaces speak the Hub's
// language: the bar is a strip with spaced-capital facts and plaques that
// press their glyphs, the queue is a ruled list under the Hub's row and
// selection washes, now playing is a sheet with a masthead and colophon, and
// every shape goes through the kit's painters so the desktop's design
// (Lithic, Material, Adwaita, Fluent) shapes it. The visualizer's bars stay
// sharp in every design.
package chrome
