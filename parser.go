package parser

type HclThinParser struct{}
type HclThinFile struct{}
type HclThinDir struct{}

func New() *HclThinParser {
	return &HclThinParser{}
}

func (HclThinParser) ReadDir(name string) *HclThinDir {
	return &HclThinDir{}
}

func (HclThinParser) ReadFile(name string) *HclThinFile {
	return &HclThinFile{}
}

func (HclThinFile) Name() string {
	return "hello"
}
