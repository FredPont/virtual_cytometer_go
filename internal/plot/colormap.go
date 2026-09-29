package plot

import (
	"image/color"
	"math"
)

// This file defines the named continuous color gradients offered by
// "Color by": one shared interpolation helper (lerpPalette) plus a few
// evenly-spaced stop sets. The same mechanism is reused, sampled at N
// evenly spaced points, to generate categorical colors for a "color by
// classes" column with more distinct values than the Okabe-Ito
// qualitative palette comfortably covers — see classColors in colorby.go.

// viridisStops: perceptually uniform, colorblind-safe — the standard
// default choice for continuous scientific data.
var viridisStops = []color.NRGBA{
	{R: 68, G: 1, B: 84, A: 255},
	{R: 59, G: 82, B: 139, A: 255},
	{R: 33, G: 145, B: 140, A: 255},
	{R: 94, G: 201, B: 98, A: 255},
	{R: 253, G: 231, B: 37, A: 255},
}

// turboStops: Google's Turbo colormap (15 evenly-spaced control points,
// values from the official reference implementation). Much higher color
// variation than Viridis across the range — this is what makes it a
// better fit than Viridis when a marker's dynamic range needs more visual
// "steps" to read clearly, at some cost to perceptual uniformity (Turbo
// is not monotonic in lightness the way Viridis is).
var turboStops = []color.NRGBA{
	{R: 48, G: 18, B: 59, A: 255},
	{R: 65, G: 69, B: 171, A: 255},
	{R: 70, G: 117, B: 237, A: 255},
	{R: 57, G: 162, B: 252, A: 255},
	{R: 27, G: 207, B: 212, A: 255},
	{R: 36, G: 236, B: 166, A: 255},
	{R: 97, G: 252, B: 108, A: 255},
	{R: 164, G: 252, B: 59, A: 255},
	{R: 209, G: 232, B: 52, A: 255},
	{R: 243, G: 198, B: 58, A: 255},
	{R: 254, G: 155, B: 45, A: 255},
	{R: 243, G: 99, B: 21, A: 255},
	{R: 217, G: 56, B: 6, A: 255},
	{R: 177, G: 25, B: 1, A: 255},
	{R: 122, G: 4, B: 2, A: 255},
}

// heatStops: a simple black -> red -> orange -> yellow -> pale "heat"
// scale, in the spirit of the White-Red / Yellow-Red single-hue gradients
// common in cytometry/expression plots.
var heatStops = []color.NRGBA{
	{R: 0, G: 0, B: 0, A: 255},
	{R: 140, G: 0, B: 0, A: 255},
	{R: 230, G: 90, B: 0, A: 255},
	{R: 255, G: 200, B: 0, A: 255},
	{R: 255, G: 255, B: 235, A: 255},
}

// GradientNames lists the gradients offered in the UI, in menu order.
var GradientNames = []string{"Viridis", "Turbo", "Heat"}

// gradientByName returns the stop set for a gradient name, defaulting to
// Viridis for an unrecognized/empty name.
func gradientByName(name string) []color.NRGBA {
	switch name {
	case "Turbo":
		return turboStops
	case "Heat":
		return heatStops
	default:
		return viridisStops
	}
}

// lerpPalette linearly interpolates a normalized value t (0..1) through a
// list of evenly-spaced color stops.
func lerpPalette(stops []color.NRGBA, t float64) color.NRGBA {
	if len(stops) == 0 {
		return color.NRGBA{}
	}
	if t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}
	pos := t * float64(len(stops)-1)
	i := int(math.Floor(pos))
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	frac := pos - float64(i)
	a, b := stops[i], stops[i+1]
	lerp := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*frac) }
	return color.NRGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

// densityColor maps a normalized intensity (0..1) to a Viridis color,
// transparent at t=0. Kept for a possible future density-heatmap display
// mode (not currently used by "color by", which calls lerpPalette
// directly so it can pick any gradient, not just Viridis).
func densityColor(t float64) color.NRGBA {
	if t <= 0 {
		return color.NRGBA{0, 0, 0, 0}
	}
	return lerpPalette(viridisStops, t)
}

// logScale maps a point count (0..max) to 0..1 on a log scale, so that a
// handful of very dense pixels doesn't visually drown out sparser areas.
func logScale(count, max uint32) float64 {
	if count == 0 || max == 0 {
		return 0
	}
	return math.Log1p(float64(count)) / math.Log1p(float64(max))
}
