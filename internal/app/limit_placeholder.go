package app

type limitNotLoadedYet struct{}

func (limitNotLoadedYet) MaxActive() (int, bool) {
	return 0, false
}
