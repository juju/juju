package mstate

type Life int8

const (
	Alive Life = iota
	Dying
	Dead
	Nlife
)

var lifeStrings = [Nlife]string{
	Alive:	"alive",
	Dying:	"dying",
	Dead:	"dead",
}

func (l Life) String() string {
	return lifeStrings[l]
}

var transitions = [Nlife][Nlife]bool{
	Alive:	{Alive: true, Dying: true},
	Dying:	{Dying: true, Dead: true},
	Dead:	{Dead: true},
}

func (l Life) isNextValid(next Life) bool {
	return transitions[l][next]
}
