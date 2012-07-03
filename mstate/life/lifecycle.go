package life

type Cycle int

const (
	Alive Cycle = 1 + iota
	Dying
	Dead
)
