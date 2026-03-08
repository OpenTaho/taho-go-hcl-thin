package parser

import (
	"bufio"
	"log"
	"math"
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

	comment        string
	docIndentation int
	docTag         string
	hclType        hcl.HclType
	nodes          []hcl.HclNode
	operator       string
	value          string
}

func (h *HclThinNode) SetDocIndentation(value int) {
	h.docIndentation = value
}

func (h *HclThinNode) SetDocTag(value string) {
	h.docTag = value
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

func (h *HclThinNode) SetBody(nodes []hcl.HclNode) {
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
	echo := &strings.Builder{}
	fileElement.SetBody(readNodes(br, echo, fileElement.Body(), false))
	nodes = append(nodes, fileElement)

	return nodes
}

func readNodes(br *bufio.Reader, echo *strings.Builder, nodes []hcl.HclNode, exitOnEOL bool) []hcl.HclNode {
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
			stringNodes := []hcl.HclNode{}
			stringNodes, err = readString(br, echo, &sb, stringNodes)
			if err != nil {
				panic(err)
			}
			ignore = true
			r = 0
		} else {
			r, _, err = br.ReadRune()
			if err != nil {
				r = 0
				willBreak = true
			}
			echo.WriteRune(r)
		}

		if r == '(' {
			ignore = true
			c = HclOther
			cc = HclUnknown
			subnodes := readNodes(br, echo, []hcl.HclNode{}, false)
			n := &HclThinNode{
				value:   "()",
				hclType: hcl.HclTypeSpan,
				nodes:   subnodes,
			}
			nodes = append(nodes, n)
		} else if r == ')' {
			nodes = addNode(nodes, hclType, sb.String())
			willBreak = true
		} else if r == '{' {
			ignore = true
			c = HclOther
			cc = HclUnknown
			subnodes := readNodes(br, echo, []hcl.HclNode{}, true)
			nodes = append(nodes, &HclThinNode{
				value:   "{}",
				hclType: hcl.HclTypeBlock,
				nodes:   subnodes,
			})
		} else if r == '}' {
			willBreak = true
			ignore = true
		} else if isToken(r) {
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
			c = HclUnknown
			cc = HclUnknown
			p, err := br.Peek(1)
			if err != nil {
				panic(err)
			}
			if p[0] == '=' {
				r, _, err = br.ReadRune()
				if err != nil {
					panic(err)
				}
				echo.WriteRune(r)
				sb.WriteRune(r)
			} else {
				ignore = true
				v := sb.String()
				sb.Reset()
				last := len(nodes) - 1
				var nType hcl.HclType
				nType = hcl.HclTypeOther
				if last > 0 {
					nType = nodes[last].Type()
					if nType == hcl.HclTypeSpace && nodes[last].Value() != "\n" {
						nodes, last = popNode(nodes, last)
						nType = nodes[last].Type()
					}
					if strings.TrimSpace(v) != "" {
						nodes = addNode(nodes, hclType, v)
						last++
					}
					n := nodes[last]
					n.SetType(hcl.HclTypePair)
					n.SetBody(readNodes(br, echo, n.Body(), true))
				}
				sb.Reset()
			}
		} else if r == '#' {
			c = HclPound
			cc = HclUnknown
			if strings.TrimSpace(sb.String()) == "" && sb.String() != "" {
				nodes = addNode(nodes, hcl.HclTypeSpace, sb.String())
				sb.Reset()
			}
			_, err = sb.WriteRune(r)
			if err != nil {
				panic(err)
			}
			hclType = hcl.HclTypeComment
			readLine(br, echo, &sb)
			ignore = true
			willBreak = exitOnEOL
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
			}

			last := len(nodes) - 1
			if last > 0 {
				if nodes[last].Value() == "<" || nodes[last-1].Value() == "<" {
					newEcho := &strings.Builder{}
					hclType, docTag := readDoc(br, newEcho, &sb)
					nodes = nodes[:last]
					last--
					n := nodes[last]
					n.SetType(hclType)
					n.SetDocTag(docTag)
					if hclType == hcl.HclTypeDocWithIndent {
						lines := strings.Split(sb.String(), "\n")
						lines = lines[:len(lines)-1]
						lead := math.MaxInt32
						for lineNum := range lines {
							line := lines[lineNum]
							lead = min(lead, len(line)-len(strings.TrimLeft(line, " ")))
						}
						n.SetDocIndentation(lead)
					}
					n.SetValue(sb.String())
					echo.WriteString(newEcho.String())
					willBreak = true
				}
			}
			sb.Reset()
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

func isToken(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_'
}

func popNode(nodes []hcl.HclNode, last int) ([]hcl.HclNode, int) {
	nodes = nodes[:last]
	last = len(nodes) - 1
	return nodes, last
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

func readDoc(br *bufio.Reader, echo *strings.Builder, sb *strings.Builder) (hcl.HclType, string) {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	echo.WriteString(s)
	sb.WriteString(s)
	docTag := strings.TrimSuffix(strings.TrimPrefix(sb.String(), "<"), "\n")
	var hclType hcl.HclType
	hclType = hcl.HclTypeDoc
	if strings.HasPrefix(docTag, "-") {
		hclType = hcl.HclTypeDocWithIndent
		docTag = strings.TrimPrefix(docTag, "-")
	}
	sb.Reset()
	for true {
		s, err = br.ReadString('\n')
		if strings.HasSuffix(s, docTag+"\n") {
			s = sb.String()
			s = strings.TrimSuffix(s, "\n")
			sb.Reset()
			sb.WriteString(s)
			break
		}
		sb.WriteString(s)
		echo.WriteString(s)
	}
	sb.WriteString("\n")
	return hclType, docTag
}

func readLine(br *bufio.Reader, echo *strings.Builder, sb *strings.Builder) {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	echo.WriteString(s)
	sb.WriteString(s)
}

func readString(br *bufio.Reader, echo *strings.Builder, sb *strings.Builder, nodes []hcl.HclNode) ([]hcl.HclNode, error) {
	escape := false
	interlop := '\x00'
	for {
		r, _, err := br.ReadRune()
		if err != nil {
			return nil, err
		}
		echo.WriteRune(r)

		if r == '\\' {
			escape = !escape
			interlop = 0
			sb.WriteRune(r)
		} else if r == '$' || r == '%' {
			if interlop != '\x00' {
				sb.WriteRune(r)
				interlop = '\x00'
			} else {
				interlop = r
			}
			sb.WriteRune(r)
		} else if interlop != 0 && r == '{' {
			interlop = 0
			newEcho := &strings.Builder{}
			nodes = readNodes(br, newEcho, nodes, false)
			echo.WriteString(newEcho.String())
			sb.WriteRune('{')
			sb.WriteString(newEcho.String())
		} else if r == '"' {
			if interlop != '\x00' {
				sb.WriteRune('$')
			}
			interlop = '\x00'
			sb.WriteRune(r)
			if !escape {
				break
			}
			escape = false
		} else {
			if interlop != '\x00' {
				sb.WriteRune('$')
			}
			interlop = '\x00'
			sb.WriteRune(r)
		}
	}
	return nodes, nil
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

func (h *HclThinNode) String() string {
	v := h.value
	switch h.hclType {
	case hcl.HclTypeString:
		v = "\"" + h.value + "\""
	case hcl.HclTypeDoc:
		v = "<<" + h.docTag + "\n" + h.value + h.docTag + "\n"
	case hcl.HclTypeDocWithIndent:
		v = "<<-" + h.docTag + "\n" + h.value + strings.Repeat(" ", h.docIndentation) + h.docTag + "\n"
	}
	return v
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
