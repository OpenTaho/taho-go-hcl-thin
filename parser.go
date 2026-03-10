package parser

import (
	"bufio"
	"cmp"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

func (h *HclThinNode) IsSimplePair() bool {
	if h.hclType != hcl.HclTypePair {
		return false
	}
	if h.comment != "" {
		return false
	}
	sb := &strings.Builder{}
	for n := range h.nodes {
		sb.WriteString(h.nodes[n].String())
	}
	v := sb.String()
	if strings.Contains(v, "\n") {
		return false
	}
	return true
}

func (h *HclThinNode) SetOperator(operator string) {
	h.operator = operator
}

func (h *HclThinNode) Operator() string {
	return h.operator
}

func (h *HclThinFile) String() string {
	sb := &strings.Builder{}
	for n := range h.body {
		sb.WriteString(h.body[n].String())
	}
	return sb.String()
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
	body []hcl.HclNode
}

func (h *HclThinFile) SaveAs(fileName string) {
	file, err := os.Create(fileName)
	if err != nil {
		return
	}
	defer file.Close()

	file.WriteString(h.String())
}

func (h *HclThinFile) Format() hcl.HclFile {
	if strings.HasSuffix(h.name, ".tfvars") {
		return &HclThinFile{
			name: h.name,
			body: formatNodes(h.body[0].Body()),
		}
	}
	return h
}

func formatNodes(nodes []hcl.HclNode) []hcl.HclNode {
	pairs := []hcl.HclNode{}

	for n := range nodes {
		node := nodes[n]
		if node.Type() == hcl.HclTypePair {
			if nodes[n-1].Type() == hcl.HclTypeComment {
				comment := nodes[n-1].Value()
				node.SetComment(comment)
			}
			pairs = append(pairs, node)
		}
	}
	fn := func(a, b hcl.HclNode) int {
		return cmp.Compare(a.Value(), b.Value())
	}
	slices.SortFunc(pairs, fn)

	simplePairs := []hcl.HclNode{}
	complexPairs := []hcl.HclNode{}

	for p := range pairs {
		if pairs[p].IsSimplePair() {
			simplePairs = append(simplePairs, pairs[p])
		} else {
			complexPairs = append(complexPairs, pairs[p])
		}
	}

	pairGroup := &HclThinNode{
		hclType: hcl.HclTypePairGroup,
		nodes: []hcl.HclNode{
			&HclThinNode{
				hclType: hcl.HclTypePairGroup,
				nodes:   simplePairs,
			},
			&HclThinNode{
				hclType: hcl.HclTypePairGroup,
				nodes:   complexPairs,
			},
		},
	}

	hasHeader := nodes[0].Type() == hcl.HclTypeComment &&
		nodes[1].Type() == hcl.HclTypeSpace &&
		nodes[1].Value() == "\n"

	if hasHeader {
		nodes = []hcl.HclNode{
			&HclThinNode{
				hclType: hcl.HclTypeSpace,
				value:   "\n",
				comment: nodes[0].Value(),
			},
			pairGroup,
		}
	} else {
		nodes = []hcl.HclNode{
			pairGroup,
		}
	}
	return nodes
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

func (_ *HclThinDir) Body() []hcl.HclNode {
	panic("not implemented")
}

func (p *HclThinParser) NewDir(name string) hcl.HclDir {
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

func (h *HclThinParser) NewFile(name string) hcl.HclFile {
	if !strings.HasPrefix(name, "/") {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		name = filepath.Join(wd, name)
	}

	body := []hcl.HclNode{}

	f, err := os.Open(name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fileElement := &HclThinNode{
		value:   filepath.Base(name),
		hclType: hcl.HclTypeBlock,
	}

	br := bufio.NewReader(f)
	echo := &strings.Builder{}
	fileElement.SetBody(readNodes(0, br, echo, fileElement.Body(), false))

	body = append(body, fileElement)

	return &HclThinFile{
		name: name,
		body: body,
	}
}

func (h *HclThinFile) Body() []hcl.HclNode {
	return h.body
}

func readNodes(endRune rune, br *bufio.Reader, echo *strings.Builder, nodes []hcl.HclNode, exitOnEOL bool) []hcl.HclNode {
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
				r = -1
				willBreak = true
			} else {
				echo.WriteRune(r)
			}
		}

		if r == '(' || r == '{' || r == '[' {
			ignore = true
			c = HclOther
			cc = HclUnknown
			endRuneMap := map[rune]rune{
				'(': ')',
				'{': '}',
				'[': ']',
			}
			subnodes := readNodes(endRuneMap[r], br, echo, []hcl.HclNode{}, false)
			n := &HclThinNode{
				value:   string(r) + string(endRuneMap[r]),
				hclType: hcl.HclTypeSpan,
				nodes:   subnodes,
			}
			nodes = append(nodes, n)
		} else if r == endRune {
			if sb.Len() > 0 {
				nodes = addNode(nodes, hclType, sb.String())
			}
			willBreak = true
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
					n.SetBody(readNodes(0, br, echo, n.Body(), true))
					body := n.Body()
					bodyLast := body[len(body)-1]
					if bodyLast.Type() == hcl.HclTypeString {
						p, err := br.Peek(1)
						if err == nil {
							if p[0] == '\n' {
								br.ReadRune()
							}
						}
					}
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
						lines := strings.Split(sb.String()+"\n", "\n")
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
			echo.WriteString(s)
			s = sb.String()
			s = strings.TrimSuffix(s, "\n")
			sb.Reset()
			sb.WriteString(s)
			break
		}
		sb.WriteString(s)
		sb.WriteString("\n")
		echo.WriteString(s)
	}
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
			nodes = readNodes('}', br, newEcho, nodes, false)
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
	switch h.hclType {
	case hcl.HclTypeString:
		return strings.TrimSuffix(strings.TrimPrefix(h.value, "\""), "\"")
	}
	return h.value
}

func (h *HclThinNode) String() string {
	sb := &strings.Builder{}
	if h.hclType == hcl.HclTypePair && !h.IsSimplePair() {
		sb.WriteRune('\n')
	}
	if h.comment != "" {
		sb.WriteString(h.comment)
	}
	switch h.hclType {
	case hcl.HclTypeBlock:
		fallthrough
	case hcl.HclTypePairGroup:
		for n := range h.nodes {
			sb.WriteString(h.nodes[n].String())
		}
	case hcl.HclTypePair:
		sb.WriteString(h.value)
		sb.WriteString(" = ")
		sbValue := &strings.Builder{}
		for n := range h.nodes {
			sbValue.WriteString(h.nodes[n].String())
		}
		sb.WriteString(strings.TrimSpace(sbValue.String()))
		sb.WriteRune('\n')
	case hcl.HclTypeDoc:
		fallthrough
	case hcl.HclTypeDocWithIndent:
		sb.WriteString("<<-")
		sb.WriteString(h.docTag)
		sb.WriteString("\n")
		sb.WriteString(h.value)
		sb.WriteString(strings.Repeat(" ", h.docIndentation))
		sb.WriteString(h.docTag)
	case hcl.HclTypeString:
		fallthrough
	case hcl.HclTypeSpace:
		fallthrough
	case hcl.HclTypeToken:
		sb.WriteString(h.value)
	case hcl.HclTypeSpan:
		sb.WriteString(h.value[0:1])
		for n := range h.nodes {
			sb.WriteString(h.nodes[n].String())
		}
		sb.WriteString(h.value[1:])
	}
	return sb.String()
}

func (h *HclThinFile) Name() string {
	return h.name
}

func New() *HclThinParser {
	return &HclThinParser{}
}
