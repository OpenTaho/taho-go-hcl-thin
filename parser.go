package parser

type HclThinParser struct{}
type HclThinFile struct{}
type HclThinDir struct{}

func New() *HclThinParser {
	return &HclThinParser{}
}

func (HclThinParser) ReadDir() *HclThinDir {
	return &HclThinDir{}
}

func (HclThinParser) ReadFile() *HclThinFile {
	return &HclThinFile{}
}

func (HclThinFile) Name() string {
	return "hello"
}
