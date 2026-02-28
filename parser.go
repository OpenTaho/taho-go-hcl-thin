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

type HclThinElement struct {
	hcl.HclElement

	isComment      bool
	isString       bool
	isHereDoc      bool
	pair           hcl.HclElement
	value          string
	nestedElements []hcl.HclElement
}

func (h *HclThinElement) NestedElements() []hcl.HclElement {
	if h.nestedElements == nil {
		return []hcl.HclElement{}
	}
	return h.nestedElements
}

func (h *HclThinElement) SetNestedElements(elements []hcl.HclElement) {
	h.nestedElements = elements
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
			m, err := regexp.MatchString(".*\\.(hcl|tf|tfvars)", name)
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

func (h *HclThinFile) Elements() []hcl.HclElement {
	elements := []hcl.HclElement{}

	f, err := os.Open(h.name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fileElement := &HclThinElement{
		value: "",
	}
	fileElement.SetNestedElements(readElements(f, err, fileElement.NestedElements()))
	elements = append(elements, fileElement)

	return elements
}

func readElements(f *os.File, err error, elements []hcl.HclElement) []hcl.HclElement {
	c := HclUnknown
	cc := HclAlphaNumericDashOrUnderscore
	ignore := false
	isComment := false
	isString := false
	pairing := false
	pairingSoon := false
	stringNext := false
	pairingNext := false
	pairingPending1 := false
	pairingPending2 := false
	hereDocExpected1 := false
	hereDocExpected2 := false
	br := bufio.NewReader(f)

	var r rune
	var sb strings.Builder

	willBreak := false
	for {
		if stringNext {
			stringNext = false
			isString = true
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

		if hereDocExpected2 {
			hereDocExpected2 = false
			ignore = true
			sb.WriteRune(r)
			readHereDoc(br, &sb)
			if pairingNext {
				pairingNext = false
				newElement := &HclThinElement{
					isHereDoc: true,
					pair:      elements[len(elements)-5],
					value:     sb.String(),
				}
				elements = pairWithLastElement(elements, newElement)
			} else {
				elements = append(elements, &HclThinElement{
					value:     sb.String(),
					isHereDoc: true,
				})
			}
			sb.Reset()
		} else if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
			c = HclAlphaNumericDashOrUnderscore
		} else if r == '"' {
			stringNext = true
			c = HclOther
			cc = HclUnknown
		} else if r == '\n' {
			c = HclNewLine
			cc = HclUnknown
		} else if unicode.IsSpace(r) {
			c = HclSpace
		} else if r == '<' {
			c = HclOther
			cc = HclUnknown
			if hereDocExpected1 {
				hereDocExpected1 = false
				hereDocExpected2 = true
			} else if pairingPending2 {
				hereDocExpected1 = true
				pairingPending2 = false
				pairingPending1 = true
			}
		} else if r == '=' {
			c = HclEquals
			cc = HclUnknown
			pairingSoon = true
		} else if r == '#' {
			c = HclPound
			cc = HclUnknown
			_, err = sb.WriteRune(r)
			if err != nil {
				panic(err)
			}
			isComment = true
			readComment(br, &sb)
			ignore = true
		} else {
			c = HclOther
			cc = HclUnknown
		}

		if cc != c {
			v := sb.String()
			if isString {
				isString = false
				v = strings.TrimPrefix(v, "\"")
				v = strings.TrimSuffix(v, "\"")
			}
			if v != "" {
				if pairing {
					pairing = false
					v = elements[len(elements)-1].Value()
					newElement := &HclThinElement{
						isComment: isComment,
						isString:  isString,
						pair:      elements[len(elements)-5],
						value:     v,
					}
					elements = pairWithLastElement(elements, newElement)
				} else {
					lastWasComment := len(elements) > 0 && elements[len(elements)-1].Comment()
					if isComment && lastWasComment {
						v = elements[len(elements)-1].Value() + v
						elements[len(elements)-1].SetValue(v)
					} else {
						elements = append(elements, &HclThinElement{
							value:     v,
							isComment: isComment,
						})
						if pairingNext {
							pairingNext = false
							pairing = true
						}
						if pairingPending2 {
							pairingPending2 = false
							pairingNext = true
						}
						if pairingPending1 {
							pairingPending1 = false
							pairingPending2 = true
						}
						if pairingSoon {
							pairingSoon = false
							pairingPending1 = true
						}
					}
				}
				sb.Reset()
			}
			cc = c
		}

		isComment = false

		if willBreak {
			break
		} else if !ignore {
			sb.WriteRune(r)
		} else {
			ignore = false
		}
	}

	return elements
}

func pairWithLastElement(elements []hcl.HclElement, newElement *HclThinElement) []hcl.HclElement {
	elements = elements[:len(elements)-4]
	last := len(elements) - 1
	elements[last] = newElement
	return elements
}

func readHereDoc(br *bufio.Reader, sb *strings.Builder) {
	s, err := br.ReadString('\n')
	if err != nil {
		panic(err)
	}
	sb.WriteString(s)
	end := strings.TrimPrefix(sb.String(), "<")
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
}

func readComment(br *bufio.Reader, sb *strings.Builder) {
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

func (h *HclThinElement) SetValue(value string) {
	h.value = value
}

func (h *HclThinElement) Comment() bool {
	return h.isComment
}

func (h *HclThinElement) Value() string {
	return h.value
}

func (h *HclThinElement) SetPair(pair hcl.HclElement) {
	h.pair = pair
}

func (h *HclThinElement) Pair() hcl.HclElement {
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
