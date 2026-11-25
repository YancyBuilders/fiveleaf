package keymap

// Preset supplies shortcuts for actions available in FiveLeaf. Other actions
// retain their defaults, and the user's bindings take precedence.
type Preset struct {
	ID       string
	Label    string
	Bindings []Entry
}
