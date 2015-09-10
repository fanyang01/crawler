package radix

type Node struct {
	child, siblings *Node
	tag             string
	end             bool
}

type Radix struct {
	root           *Node
	containZeroStr bool
	size           int
}

func NewRadix() *Radix {
	return &Radix{}
}

func (rt *Radix) Insert(s string) {
	if s == "" {
		if !rt.containZeroStr {
			rt.containZeroStr = true
			rt.size++
		}
		return
	}

	pp, node := &rt.root, rt.root
	i := 0
LOOP:
	for {
		if node == nil {
			node = &Node{
				tag: s[i:],
			}
			*pp = node
			break
		}

		j := 0
		for j < len(node.tag) && i < len(s) && s[i] == node.tag[j] {
			i, j = i+1, j+1
		}

		switch j {
		case len(node.tag):
		case 0:
			for node != nil && node.tag[0] < s[i] {
				pp, node = &node.siblings, node.siblings
			}
			if node == nil || node.tag[0] == s[i] {
				continue LOOP
			}
			node = &Node{
				siblings: node,
				tag:      s[i:],
			}
			*pp = node
			break LOOP
		default:
			suffix := node.tag[j:]
			node.tag = node.tag[:j]
			node.child = &Node{
				child: node.child,
				tag:   suffix,
				end:   node.end,
			}
		}

		if i < len(s) {
			pp, node = &node.child, node.child
		} else {
			break
		}
	}

	if node != nil {
		node.end = true
		rt.size++
	}
}

func (rt *Radix) Lookup(s string) bool {
	if s == "" {
		return rt.containZeroStr
	}

	node, i := rt.root, 0
	for {
		if node == nil {
			return false
		}

		j := 0
		for j < len(node.tag) && i < len(s) && s[i] == node.tag[j] {
			i, j = i+1, j+1
		}

		switch j {
		case len(node.tag):
		case 0:
			for node != nil && node.tag[0] < s[i] {
				node = node.siblings
			}
			if node == nil || node.tag[0] != s[i] {
				return false
			}
			continue
		default:
			return false
		}

		if i < len(s) {
			node = node.child
		} else {
			break
		}
	}
	return node.end
}
