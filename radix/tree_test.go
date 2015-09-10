package radix

import (
	"fmt"
	"log"
	"strings"
	"testing"
)

var (
	article = `In computer science, a trie, also called digital tree and sometimes radix tree or prefix tree`

	// Bitwise tries are much the same as a normal character based trie except that individual bits are used to traverse what effectively becomes a form of binary tree. Generally, implementations use a special CPU instruction to very quickly find the first set bit in a fixed length key (e.g., GCC's __builtin_clz() intrinsic). This value is then used to index a 32- or 64-entry table which points to the first item in the bitwise trie with that number of leading zero bits. The search then proceeds by testing each subsequent bit in the key and choosing child[0] or child[1] appropriately until the item is found.
	// Although this process might sound slow, it is very cache-local and highly parallelizable due to the lack of register dependencies and therefore in fact has excellent performance on modern out-of-order execution CPUs. A red-black tree for example performs much better on paper, but is highly cache-unfriendly and causes multiple pipeline and TLB stalls on modern CPUs which makes that algorithm bound by memory latency rather than CPU speed. In comparison, a bitwise trie rarely accesses memory and when it does it does so only to read, thus avoiding SMP cache coherency overhead. Hence, it is increasingly becoming the algorithm of choice for code that performs many rapid insertions and deletions, such as memory allocators (e.g., recent versions of the famous Doug Lea's allocator (dlmalloc) and its descendents).`
	words = strings.Fields(article)
)

func TestInsert(t *testing.T) {
	ws := []string{
		"hello",
		"",
		"world",
		"last",
		"letter",
		"least",
		"low",
		"l",
		"L",
		"h",
		"he",
		"world",
		"hello",
		"l",
		"lower",
	}

	rt := NewRadix()
	for _, word := range ws {
		rt.Insert(word)
	}

	for _, word := range ws {
		if ok := rt.Lookup(word); !ok {
			log.Println(word)
			t.Fail()
		}
	}
	// printSibling(rt.root)
	for _, word := range words {
		rt.Insert(word)
	}

	for _, word := range words {
		if ok := rt.Lookup(word); !ok {
			log.Println(word)
			t.Fail()
		}
	}
}

func printSibling(node *Node) {
	for n := node; n != nil; n = n.siblings {
		fmt.Printf("%s ", n.tag)
	}
	fmt.Println("")
	for n := node; n != nil; n = n.siblings {
		fmt.Printf("%s: ", n.tag)
		printSibling(n.child)
	}
}

func BenchmarkLookup(b *testing.B) {
	rt := NewRadix()
	for _, word := range words {
		rt.Insert(word)
	}

	for i := 0; i < b.N; i++ {
		present := rt.Lookup(words[i%len(words)])
		present = !present
	}
}

func BenchmarkMap(b *testing.B) {
	mp := make(map[string]bool)
	for _, word := range words {
		mp[word] = true
	}

	for i := 0; i < b.N; i++ {
		present := mp[words[i%len(words)]]
		present = !present
	}
}
