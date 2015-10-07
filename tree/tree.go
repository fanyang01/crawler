package tree

type nodeType int

const (
	cNode nodeType = iota // common
	wNode                 // wildcard character
)

type Tree struct {
	root *Node
}

type Node struct {
	child    []*Node
	childidx []byte // first byte of each child
	wcard    *Node
	s        string
	v        interface{}
	typ      nodeType
	end      bool
}

func newTree(pattern string, v interface{}) *Node {
	var root, n, child *Node
	var j int
	for i := 0; i < len(pattern); {
		s, escape := []byte{}, false
	FIND_AST:
		for j = 0; j < len(pattern[i:]); j++ {
			switch pattern[i+j] {
			case '\\':
				if escape = !escape; escape {
					continue FIND_AST
				}
			case '*':
				if !escape {
					break FIND_AST
				}
			}
			escape = false
			s = append(s, pattern[i+j])
		}
		switch j {
		case 0:
			child = &Node{
				s:   "*",
				typ: wNode,
			}
			i++
		default:
			child = &Node{
				s:   string(s),
				typ: cNode,
			}
			i = i + j
		}
		if n != nil {
			switch child.typ {
			case wNode:
				n.wcard = child
			case cNode:
				n.child = []*Node{child}
				n.childidx = []byte{child.s[0]}
			}
		} else {
			root = child
		}
		n = child
	}
	n.v = v
	n.end = true
	return root
}

func (n *Node) setV(v interface{}) (ov interface{}, is bool) {
	ov, is = n.v, n.end
	n.v, n.end = v, true
	return
}

func (t *Tree) Add(pattern string, v interface{}) (ov interface{}, is bool) {
	if pattern == "" {
		return
	}
	if t.root == nil {
		t.root = newTree(pattern, v)
		return
	}
	n := t.root
INSERT:
	for {
		var i, l int
		var wmatch, escape bool

		if n.typ == wNode {
			if len(pattern) > 0 && pattern[0] == '*' {
				wmatch = true
			}
			goto SWITCH
		}

		for i < len(pattern) && l < len(n.s) {
			if pattern[i] == '\\' {
				if escape = !escape; escape {
					i++
					continue
				}
			}
			if !escape && pattern[i] == '*' {
				break
			}
			if pattern[i] != n.s[l] {
				break
			}
			escape = false
			i, l = i+1, l+1
		}
		if escape {
			i--
			escape = false
		}
	SWITCH:
		switch {
		case wmatch:
			i = 1
			fallthrough
		case l == len(n.s): // totally match this node
			pattern = pattern[i:]
			if len(pattern) == 0 { // end
				return n.setV(v)
			}
			if pattern[0] == '*' {
				if n.wcard == nil {
					n.wcard = newTree(pattern, v)
					return
				} else {
					n = n.wcard
					continue INSERT
				}
			}

			first := 0
			if pattern[0] == '\\' {
				first = 1
			}
			if len(pattern[first:]) > 0 {
				for i := 0; i < len(n.childidx); i++ {
					if n.childidx[i] == pattern[first] {
						n = n.child[i]
						continue INSERT
					}
				}
			}
			// not found
		case n.typ == wNode:
			i, l = 0, 0
			fallthrough
		default: // split
			prefix, suffix := n.s[:l], n.s[l:]
			child := &Node{
				s:        suffix,
				typ:      n.typ,
				child:    n.child,
				childidx: n.childidx,
				wcard:    n.wcard,
			}
			*n = Node{}
			n.s = prefix
			n.typ = cNode
			if child.typ == wNode {
				n.wcard = child
			} else {
				n.child = []*Node{child}
				n.childidx = []byte{child.s[0]}
			}
			pattern = pattern[i:]
			if len(pattern) == 0 { // end
				return n.setV(v)
			}
		}
		// construct a new subtree using rest of pattern and
		// append it to the child list of this node
		child := newTree(pattern, v)
		switch child.typ {
		case cNode:
			n.child = append(n.child, child)
			n.childidx = append(n.childidx, child.s[0])
		case wNode:
			n.wcard = child
		}
		return
	}
}

func (t *Tree) Lookup(s string) (v interface{}, ok bool) {
	n := lookup(t.root, s)
	if n != nil {
		v, ok = n.v, n.end
	}
	return
}

func lookup(n *Node, s string) *Node {
	if n == nil {
		return nil
	}
	if n.typ == wNode {
		for capture := 0; capture <= len(s); capture++ {
			if end := lookupW(n, s[capture:]); end != nil {
				return end
			}
		}
		return nil
	}

	minLen := len(s)
	if minLen > len(n.s) {
		minLen = len(n.s)
	}
	var l int // length of longest common prefix
	for l = 0; l < minLen && s[l] == n.s[l]; l++ {
	} // at the end of loop: pattern[:l] == n.s[:l]
	switch l {
	case len(n.s): // totally match this node
		s = s[l:]
		if len(s) == 0 { // end
			if end := lookup(n.wcard, s); end != nil {
				return end
			}
			return n
		}
		// go down
		var k int
		for k = 0; k < len(n.child); k++ {
			if n.child[k].s[0] == s[0] {
				if end := lookup(n.child[k], s); end != nil {
					return end
				}
				break
			}
		}
		// try '*'
		return lookup(n.wcard, s)
	default:
		return nil
	}
}

// n must be a wildcard node
func lookupW(n *Node, s string) *Node {
	if s == "" {
		return n
	}
	var end *Node
	for i := 0; i < len(n.childidx); i++ {
		if n.childidx[i] == s[0] {
			if end = lookup(n.child[i], s); end != nil {
				return end
			}
			break
		}
	}
	// try '*'
	return lookup(n.wcard, s)
}
