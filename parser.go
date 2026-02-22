package parser

import hcl "github.com/openTaho/taho-go-hcl"

type HclThinParser struct{}
type HclThinFile struct{}
type HclThinDir struct{}

func New() *HclThinParser {
	return &HclThinParser{}
}

func (HclThinParser) ReadDir(name string) hcl.HclDir {
	return &HclThinDir{}
}

func (HclThinParser) ReadFile(name string) hcl.HclDir {
	return &HclThinFile{}
}

func (HclThinFile) Name() string {
	return "hello"
}
