package jujusvg

import (
	"image"
	"math"
	"sort"
)

// getPointOutside returns a point that is outside the hull of existing placed
// vertices so that an object can be placed on the canvas without overlapping
// others.
func getPointOutside(vertices []image.Point, padding image.Point) image.Point {
	// Shortcut some easy solutions.
	switch len(vertices) {
	case 0:
		return image.Point{0, 0}
	case 1:
		return image.Point{
			vertices[0].X + padding.X,
			vertices[0].Y + padding.Y,
		}
	case 2:
		return image.Point{
			int(math.Max(float64(vertices[0].X), float64(vertices[1].X))) + padding.X,
			int(math.Max(float64(vertices[0].Y), float64(vertices[1].Y))) + padding.Y,
		}
	}
	hull := convexHull(vertices)
	// Find point that is the furthest to the right on the hull.
	var rightmost image.Point
	maxDistance := 0.0
	for _, vertex := range hull {
		fromOrigin := line{p0: vertex, p1: image.Point{0, 0}}
		distance := fromOrigin.length()
		if math.Abs(distance) > maxDistance {
			maxDistance = math.Abs(distance)
			rightmost = vertex
		}
	}
	return image.Point{
		rightmost.X + padding.X,
		rightmost.Y + padding.Y,
	}
}

// vertexSet implements sort.Interface for image.Point, sorting first by X, then
// by Y
type vertexSet []image.Point

func (vs vertexSet) Len() int      { return len(vs) }
func (vs vertexSet) Swap(i, j int) { vs[i], vs[j] = vs[j], vs[i] }
func (vs vertexSet) Less(i, j int) bool {
	if vs[i].X == vs[j].X {
		return vs[i].Y < vs[j].Y
	}
	return vs[i].X < vs[j].X
}

// convexHull takes a list of vertices and returns the set of vertices which
// make up the convex hull encapsulating all vertices on a plane.
func convexHull(vertices []image.Point) []image.Point {
	// Simple cases can be shortcutted.
	if len(vertices) == 0 {
		return []image.Point{
			{0, 0},
		}
	}
	// For our purposes, we can assume that three vertices form a hull.
	if len(vertices) < 4 {
		return vertices
	}

	sort.Sort(vertexSet(vertices))
	var lower, upper []image.Point
	for _, vertex := range vertices {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], vertex) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, vertex)
	}

	for _, vertex := range reverse(vertices) {
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], vertex) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, vertex)
	}
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

// cross finds the 2D cross-product of OA and OB vectors.
// Returns a positive value if OAB makes a counter-clockwise turn, a negative
// value if OAB makes a clockwise turn, and zero if the points are collinear.
func cross(o, a, b image.Point) int {
	return (a.X-o.X)*(b.Y-o.Y) - (a.Y-o.Y)*(b.X-o.X)
}

// reverse reverses a slice of Points for use in finding the upper hull.
func reverse(vertices []image.Point) []image.Point {
	for i := 0; i < len(vertices)/2; i++ {
		opp := len(vertices) - (i + 1)
		vertices[i], vertices[opp] = vertices[opp], vertices[i]
	}
	return vertices
}
