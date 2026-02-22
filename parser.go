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

func (*HclThinDir) Name() string {
	return "hello dir"
}

func (*HclThinDir) SetName(value string) {
	panic("not implemented")
}

func (HclThinParser) ReadFile(name string) hcl.HclFile {
	return &HclThinFile{}
}

func (*HclThinDir) Files() []hcl.HclFile {
	panic("not implemented")
}

func (*HclThinFile) Elements() []hcl.HclElement {
	panic("not implemented")
}

func (*HclThinFile) Name() string {
	return "hello file"
}

func (*HclThinFile) SetName(value string) {
	panic("not implemented")
}
