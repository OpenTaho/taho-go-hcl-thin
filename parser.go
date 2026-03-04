package parser

import (
	"bufio"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	hcl "github.com/openTaho/taho-go-hcl"
)

const (
	HclAlphaNumericDashOrUnderscore = iota
	HclEquals
	HclNewLine
	HclOther
	HclPound
	HclSpace
	HclUnknown
)

type HclThinDir struct {
	name   string
	parser hcl.HclParser
}

type HclThinNode struct {
	hcl.HclNode

	comment  string
	hclType  hcl.HclType
	nodes    []hcl.HclNode
	operator string
	pair     hcl.HclNode
	value    string
}

func (h *HclThinNode) SetType(value hcl.HclType) {
	h.hclType = value
}

func (h *HclThinNode) SetOperator(operator string) {
	h.operator = operator
}

func (h *HclThinNode) Operator() string {
	return h.operator
}

func (h *HclThinNode) Body() []hcl.HclNode {
	return h.nodes
}

func (h *HclThinNode) SetNodes(nodes []hcl.HclNode) {
	h.nodes = nodes
}

func (h *HclThinNode) Type() hcl.HclType {
	return h.hclType
}

type HclThinFile struct {
	name string
}

type HclThinParser struct{}

func (h *HclThinDir) Files() ([]hcl.HclFile, error) {
	files, err := os.ReadDir(h.name)
	if err != nil {
		return nil, err
	}

	hclFiles := []hcl.HclFile{}
	for _, file := range files {
		name := file.Name()
		if !file.IsDir() {
			m, err := regexp.MatchString(".*\\.(hcl|tf|tfvars)$", name)
			if err != nil {
				panic(err)
			}
			if m {
				hclFile := h.parser.NewFile(filepath.Join(h.name, name))
				hclFiles = append(hclFiles, hclFile)
			}
		}
	}

	return hclFiles, err
}

func (h *HclThinDir) Name() string {
	return h.name
}

func (_ *HclThinDir) SetName(value string) {
	panic("not implemented")
}

func (_ *HclThinDir) Body() []hcl.HclNode {
	panic("not implemented")
}

func (p HclThinParser) NewDir(name string) hcl.HclDir {
	if !strings.HasPrefix(name, "/") {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		name = filepath.Join(wd, name)
	}

	return &HclThinDir{
		name:   name,
		parser: p,
	}
}

func (HclThinParser) NewFile(name string) hcl.HclFile {
	if !strings.HasPrefix(name, "/") {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		name = filepath.Join(wd, name)
	}

	return &HclThinFile{
		name: name,
	}
}

func (h *HclThinFile) Body() []hcl.HclNode {
	nodes := []hcl.HclNode{}

	f, err := os.Open(h.name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fileElement := &HclThinNode{
		value: filepath.Base(h.name),
	}
	br := bufio.NewReader(f)
	fileElement.SetNodes(readNodes(br, fileElement.Body(), false))
	nodes = append(nodes, fileElement)

	return nodes
}

func readNodes(br *bufio.Reader, nodes []hcl.HclNode, exitOnEOL bool) []hcl.HclNode {
	c := HclUnknown
	cc := HclAlphaNumericDashOrUnderscore
	var hclType hcl.HclType
	hclType = hcl.HclTypeOther
	ignore := false
	stringNext := false

	var r rune
	var err error
	var sb strings.Builder

	willBreak := false
	for {
		if stringNext {
			stringNext = false
			hclType = hcl.HclTypeString
			readString(br, &sb)
			ignore = true
			r = 0
		} else {
			r, _, err = br.ReadRune()
			if err != nil {
				r = ' '
				willBreak = true
			}
		}

		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
			c = HclAlphaNumericDashOrUnderscore
		} else if r == '"' {
			stringNext = true
			c = HclOther
			cc = HclUnknown
		} else if r == '\n' {
			c = HclNewLine
			cc = HclUnknown
			if exitOnEOL {
				willBreak = true
			}
		} else if unicode.IsSpace(r) {
			c = HclSpace
		} else if r == '=' {
			// Read from here to the end of the line to parse expressions
			//
			// If we encounter parentheses, brackets, or braces we will need to parse
			// those appropiatly.
			c = HclUnknown
			cc = HclUnknown
			ignore = true
			v := sb.String()
			if v != "" {
				nodes = addNode(nodes, hclType, v)
			}
			sb.Reset()
			last := len(nodes) - 1
			nType := nodes[last].Type()
			var pop bool
			comment := ""
			if nType == hcl.HclTypeComment {
				comment = nodes[last].Value()
				pop = true
			} else {
				pop = nType == hcl.HclTypeSpace
			}
			if pop {
				nodes = nodes[:last]
				last = len(nodes) - 1
			}
			n := nodes[last]
			n.SetComment(comment)
			n.SetType(hcl.HclTypePair)
			n.SetNodes(readNodes(br, n.Body(), true))
			sb.Reset()
		} else if r == '#' {
			c = HclPound
			cc = HclUnknown
			_, err = sb.WriteRune(r)
			if err != nil {
				panic(err)
			}
			hclType = hcl.HclTypeComment
			readLine(br, &sb)
			ignore = true
		} else {
			c = HclOther
			cc = HclUnknown
		}

		if cc != c {
			v := sb.String()
			if hclType == hcl.HclTypeString {
				v = strings.TrimPrefix(v, "\"")
				v = strings.TrimSuffix(v, "\"")
			}

			if v != "" {
				nodes = addNode(nodes, hclType, v)
				last := len(nodes) - 1
				if last > 0 {
					if nodes[last].Value() == "<" || nodes[last-1].Value() == "<" {
						hclType = readDoc(br, &sb)
						nodes = nodes[:last]
						last = len(nodes) - 1
						nodes[last].SetType(hclType)
						nodes[last].SetValue(sb.String())
						willBreak = true
					}
				}
				sb.Reset()
			}
			cc = c
		}

		hclType = hcl.HclTypeToken

		if willBreak {
			break
		} else if !ignore {
			sb.WriteRune(r)
		} else {
			ignore = false
		}
	}

	return nodes
}

func addNode(nodes []hcl.HclNode, hclType hcl.HclType, v string) []hcl.HclNode {
	nLen := len(nodes)
	last := nLen - 1
	lastWasComment := nLen > 0 && nodes[last].Type() == hcl.HclTypeComment
	if hclType == hcl.HclTypeComment && lastWasComment {
		v = nodes[last].Value() + v
		nodes[last].SetValue(v)
	} else {
		if strings.TrimSpace(v) == "" {
			hclType = hcl.HclTypeSpace
		}
		nodes = append(nodes, &HclThinNode{
			value:   v,
			hclType: hclType,
		})
	}
	return nodes
}

func readDoc(br *bufio.Reader, sb *strings.Builder) hcl.HclType {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
	end := strings.TrimPrefix(sb.String(), "<")
	var hclType hcl.HclType
	hclType = hcl.HclTypeHereDoc
	if strings.HasPrefix(end, "-") {
		hclType = hcl.HclTypeHereDocWithIndent
		end = strings.TrimPrefix(end, "-")
	}
	sb.Reset()
	for true {
		s, err = br.ReadString('\n')
		if strings.HasSuffix(s, end) {
			s = sb.String()
			s = strings.TrimSuffix(s, "\n")
			sb.Reset()
			sb.WriteString(s)
			break
		}
		sb.WriteString(s + "\n")
	}
	return hclType
}

func readLine(br *bufio.Reader, sb *strings.Builder) {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
}

func readString(br *bufio.Reader, sb *strings.Builder) {
	s, err := br.ReadString('"')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
	if strings.HasSuffix(s, "\\\"") {
		readString(br, sb)
	}
}

func (h *HclThinNode) SetComment(value string) {
	h.comment = value
}

func (h *HclThinNode) SetValue(value string) {
	h.value = value
}

func (h *HclThinNode) Value() string {
	return h.value
}

func (h *HclThinNode) SetPair(pair hcl.HclNode) {
	h.pair = pair
}

func (h *HclThinNode) Pair() hcl.HclNode {
	return h.pair
}

func (h *HclThinFile) Name() string {
	return h.name
}

func (*HclThinFile) SetName(value string) {
	panic("not implemented")
}

func New() *HclThinParser {
	return &HclThinParser{}
}
