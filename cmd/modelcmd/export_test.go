package modelcmd

var NewAPIContext = newAPIContext

func SetRunStarted(b interface {
	setRunStarted()
}) {
	b.setRunStarted()
}
