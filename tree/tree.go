package tree

type nodeType int

const (
	oNode nodeType = iota // ordinary
	wNode                 // wildcard
	eNode                 // end
)

type Tree struct {
	root *Node
}

type Node struct {
	child []*Node
	index []byte // first byte of each child
	s     string
	v     interface{}
	typ   nodeType
}

func newSubtree(pattern string, v interface{}) *Node {
	var root, n, child *Node
	var j int
	for i := 0; i < len(pattern); {
		for j = 0; j < len(pattern[i:]) && pattern[i+j] != '*'; j++ {
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
				s:   pattern[i : i+j],
				typ: oNode,
			}
			i = i + j
		}
		if n != nil {
			n.child = []*Node{child}
			n = child
		} else {
			n = child
			root = n
		}
	}
	n.v = v
	return root
}

// panic("a registered handler conflicts with pattern: " + pattern[:i+l])
func (t *Tree) Insert(pattern string, v interface{}) {
	if t.root == nil {
		t.root = newSubtree(pattern, v)
		return
	}
	n := t.root
INSERT:
	for {
		minLen := len(pattern)
		if minLen > len(n.s) {
			minLen = len(n.s)
		}
		var l int // length of longest common prefix
		for l = 0; l < minLen && pattern[l] == n.s[l]; l++ {
		} // at the end of loop: pattern[:l] == n.s[:l]
		switch l {
		case len(n.s): // totally match this node
			pattern = pattern[l:]
			if len(pattern) == 0 { // end
				n.v = v
				break INSERT
			}
			// go down
			var k int
			for k = 0; k < len(n.child); k++ {
				if n.child[k].s[0] == pattern[0] {
					break
				}
			}
			if k != len(n.child) { // found
				n = n.child[k]
				continue INSERT
			} // else not found
		default: // split
			child := &Node{
				s:     n.s[l:],
				typ:   oNode,
				child: n.child,
			}
			n.s = n.s[:l]
			n.child = []*Node{child}
			pattern = pattern[l:]
			if len(pattern) == 0 { // end
				n.v = v
				break INSERT
			}
		}
		// construct a new subtree using rest of pattern and
		// append it to the child list of this node
		n.child = append(n.child, newSubtree(pattern, v))
		break INSERT
	}
}

func (t *Tree) Lookup(s string) *Node {
	return lookup(t.root, s)
}

func lookup(n *Node, s string) *Node {
	if n == nil {
		return nil
	}
	var found *Node
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
			return n
		}
		// go down
		var k int
		for k = 0; k < len(n.child); k++ {
			if n.child[k].s[0] == s[0] {
				found = lookup(n.child[k], s)
				break
			}
		}
		if found != nil {
			return found
		}
		// try '*'
		for k = 0; k < len(n.child); k++ {
			if n.child[k].s[0] == '*' {
				return lookup(n.child[k], s)
			}
		}
		return nil
	default:
		if l != 0 || n.s[0] != '*' {
			return nil
		}
		if len(s) == 0 {
			return n
		}
		for catch := 0; catch < len(s); catch++ {
			if found = lookupW(n, s[catch:]); found != nil {
				return found
			}
		}
		return nil
	}
}

func lookupW(n *Node, s string) *Node {
	var found *Node
	for i := 0; i < len(n.child); i++ {
		if n.child[i].s[0] == s[0] {
			if found = lookup(n.child[i], s); found != nil {
				return found
			}
			break
		}
	}
	// try '*'
	for i := 0; i < len(n.child); i++ {
		if n.child[i].s[0] == '*' {
			return lookup(n.child[i], s)
		}
	}
	return nil
}
