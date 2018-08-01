package jujusvg

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"math"

	svg "github.com/ajstarks/svgo"

	"gopkg.in/juju/jujusvg.v3/assets"
)

const (
	iconSize             = 96
	applicationBlockSize = 180
	healthCircleRadius   = 8
	relationLineWidth    = 1
	maxInt               = int(^uint(0) >> 1)
	minInt               = -(maxInt - 1)
	maxHeight            = 450
	maxWidth             = 1000

	fontColor     = "#505050"
	relationColor = "#a7a7a7"
)

// Canvas holds the parsed form of a bundle or model.
type Canvas struct {
	applications  []*application
	relations     []*applicationRelation
	iconsRendered map[string]bool
	iconIds       map[string]string
}

// application represents a application deployed to a model and contains the
// point of the top-left corner of the icon, icon URL, and additional metadata.
type application struct {
	name      string
	charmPath string
	iconUrl   string
	iconSrc   []byte
	point     image.Point
}

// applicationRelation represents a relation created between two applications.
type applicationRelation struct {
	name         string
	applicationA *application
	applicationB *application
}

// line represents a line segment with two endpoints.
type line struct {
	p0, p1 image.Point
}

// definition creates any necessary defs that can be used later in the SVG.
func (s *application) definition(canvas *svg.SVG, iconsRendered map[string]bool, iconIds map[string]string) error {
	if len(s.iconSrc) == 0 || iconsRendered[s.charmPath] {
		return nil
	}
	iconsRendered[s.charmPath] = true
	iconIds[s.charmPath] = fmt.Sprintf("icon-%d", len(iconsRendered))

	// Temporary solution:
	iconBuf := bytes.NewBuffer(s.iconSrc)
	return processIcon(iconBuf, canvas.Writer, iconIds[s.charmPath])
}

// usage creates any necessary tags for actually using the application in the SVG.
func (s *application) usage(canvas *svg.SVG, iconIds map[string]string) {
	canvas.Group(fmt.Sprintf(`transform="translate(%d,%d)"`, s.point.X, s.point.Y))
	defer canvas.Gend()
	canvas.Title(s.name)
	canvas.Circle(
		applicationBlockSize/2,
		applicationBlockSize/2,
		applicationBlockSize/2,
		`class="application-block" fill="#f5f5f5" stroke="#888" stroke-width="1"`)
	if len(s.iconSrc) > 0 {
		canvas.Use(
			0,
			0,
			"#"+iconIds[s.charmPath],
			fmt.Sprintf(`transform="translate(%d,%d)" width="%d" height="%d" clip-path="url(#clip-mask)"`, applicationBlockSize/2-iconSize/2, applicationBlockSize/2-iconSize/2, iconSize, iconSize),
		)
	} else {
		canvas.Image(
			applicationBlockSize/2-iconSize/2,
			applicationBlockSize/2-iconSize/2,
			iconSize,
			iconSize,
			s.iconUrl,
			`clip-path="url(#clip-mask)"`,
		)
	}
	name := s.name
	if len(name) > 20 {
		name = fmt.Sprintf("%s...", name[:17])
	}
	canvas.Rect(
		0,
		applicationBlockSize-45,
		applicationBlockSize,
		32,
		`rx="2" ry="2" fill="rgba(220, 220, 220, 0.8)"`)
	canvas.Text(
		applicationBlockSize/2,
		applicationBlockSize-23,
		name,
		`text-anchor="middle" style="font-weight:200"`)
}

// definition creates any necessary defs that can be used later in the SVG.
func (r *applicationRelation) definition(canvas *svg.SVG) {
}

// usage creates any necessary tags for actually using the relation in the SVG.
func (r *applicationRelation) usage(canvas *svg.SVG) {
	canvas.Group()
	defer canvas.Gend()
	canvas.Title(r.name)
	l := line{
		p0: r.applicationA.point.Add(point(applicationBlockSize/2, applicationBlockSize/2)),
		p1: r.applicationB.point.Add(point(applicationBlockSize/2, applicationBlockSize/2)),
	}
	canvas.Line(
		l.p0.X,
		l.p0.Y,
		l.p1.X,
		l.p1.Y,
		fmt.Sprintf(`stroke=%q`, relationColor),
		fmt.Sprintf(`stroke-width="%dpx"`, relationLineWidth),
		fmt.Sprintf(`stroke-dasharray=%q`, strokeDashArray(l)),
	)
	mid := l.p0.Add(l.p1).Div(2).Sub(point(healthCircleRadius, healthCircleRadius))
	canvas.Use(mid.X, mid.Y, "#healthCircle")

	deg := math.Atan2(float64(l.p0.Y-l.p1.Y), float64(l.p0.X-l.p1.X))
	canvas.Circle(
		int(float64(l.p0.X)-math.Cos(deg)*(applicationBlockSize/2)),
		int(float64(l.p0.Y)-math.Sin(deg)*(applicationBlockSize/2)),
		4,
		fmt.Sprintf(`fill=%q`, relationColor))
	canvas.Circle(
		int(float64(l.p1.X)+math.Cos(deg)*(applicationBlockSize/2)),
		int(float64(l.p1.Y)+math.Sin(deg)*(applicationBlockSize/2)),
		4,
		fmt.Sprintf(`fill=%q`, relationColor))
}

// strokeDashArray generates the stroke-dasharray attribute content so that
// the relation health indicator is placed in an empty space.
func strokeDashArray(l line) string {
	return fmt.Sprintf("%.2f, %d", l.length()/2-healthCircleRadius, healthCircleRadius*2)
}

// length calculates the length of a line.
func (l *line) length() float64 {
	dp := l.p0.Sub(l.p1)
	return math.Sqrt(square(float64(dp.X)) + square(float64(dp.Y)))
}

// addApplication adds a new application to the canvas.
func (c *Canvas) addApplication(s *application) {
	c.applications = append(c.applications, s)
}

// addRelation adds a new relation to the canvas.
func (c *Canvas) addRelation(r *applicationRelation) {
	c.relations = append(c.relations, r)
}

// layout adjusts all items so that they are positioned appropriately,
// and returns the overall size of the canvas.
func (c *Canvas) layout() (int, int) {
	minWidth := maxInt
	minHeight := maxInt
	maxWidth := minInt
	maxHeight := minInt

	for _, application := range c.applications {
		if application.point.X < minWidth {
			minWidth = application.point.X
		}
		if application.point.Y < minHeight {
			minHeight = application.point.Y
		}
		if application.point.X > maxWidth {
			maxWidth = application.point.X
		}
		if application.point.Y > maxHeight {
			maxHeight = application.point.Y
		}
	}
	for _, application := range c.applications {
		application.point = application.point.Sub(point(minWidth, minHeight))
	}
	return abs(maxWidth-minWidth) + applicationBlockSize + 1,
		abs(maxHeight-minHeight) + applicationBlockSize + 1
}

func (c *Canvas) definition(canvas *svg.SVG) {
	canvas.Def()
	defer canvas.DefEnd()

	// Relation health circle.
	canvas.Group(`id="healthCircle"`,
		`transform="scale(1.1)"`)
	io.WriteString(canvas.Writer, assets.RelationIconHealthy)
	canvas.Gend()

	// Application and relation specific defs.
	for _, relation := range c.relations {
		relation.definition(canvas)
	}
	for _, application := range c.applications {
		application.definition(canvas, c.iconsRendered, c.iconIds)
	}
}

func (c *Canvas) relationsGroup(canvas *svg.SVG) {
	canvas.Gid("relations")
	defer canvas.Gend()
	for _, relation := range c.relations {
		relation.usage(canvas)
	}
}

func (c *Canvas) applicationsGroup(canvas *svg.SVG) {
	canvas.Gid("applications")
	defer canvas.Gend()
	for _, application := range c.applications {
		application.usage(canvas, c.iconIds)
	}
}

func (c *Canvas) iconClipPath(canvas *svg.SVG) {
	canvas.Circle(
		applicationBlockSize/2-iconSize/2+5, // for these two, add an offset to help
		applicationBlockSize/2-iconSize/2+7, // hide the embossed border.
		applicationBlockSize/4,
		`id="application-icon-mask" fill="none"`)
	canvas.ClipPath(`id="clip-mask"`)
	defer canvas.ClipEnd()
	canvas.Use(
		0,
		0,
		`#application-icon-mask`)
}

// Marshal renders the SVG to the given io.Writer.
func (c *Canvas) Marshal(w io.Writer) {
	// Initialize maps for application icons, which are used both in definition
	// and use methods for applications.
	c.iconsRendered = make(map[string]bool)
	c.iconIds = make(map[string]string)

	// TODO check write errors and return an error from
	// Marshal if the write fails. The svg package does not
	// itself check or return write errors; a possible work-around
	// is to wrap the writer in a custom writer that panics
	// on error, and catch the panic here.
	width, height := c.layout()

	canvas := svg.New(w)
	canvas.Start(
		width,
		height,
		fmt.Sprintf(`style="font-family:Ubuntu, sans-serif;" viewBox="0 0 %d %d"`,
			width, height),
	)
	defer canvas.End()
	c.definition(canvas)
	c.iconClipPath(canvas)
	c.relationsGroup(canvas)
	c.applicationsGroup(canvas)
}

// abs returns the absolute value of a number.
func abs(x int) int {
	if x < 0 {
		return -x
	} else {
		return x
	}
}

// square multiplies a number by itself.
func square(x float64) float64 {
	return x * x
}

// point generates an image.Point given its coordinates.
func point(x, y int) image.Point {
	return image.Point{x, y}
}
