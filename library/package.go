package library

type Package struct {
	Name     string
	Version  string
	As       string
	FileName string
	Raw      string
	cached   bool
}
