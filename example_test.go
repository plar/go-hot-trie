package hot_test

import (
	"fmt"

	hot "github.com/plar/go-hot-trie"
)

func Example() {
	tree := hot.New()
	tree.Insert(hot.Key("apple"), "red")
	tree.Insert(hot.Key("banana"), "yellow")
	tree.Insert(hot.Key("cherry"), "dark red")

	if value, found := tree.Search(hot.Key("banana")); found {
		fmt.Println("banana is", value)
	}
	// Output: banana is yellow
}

func ExampleTree_ForEach() {
	tree := hot.New()
	for _, w := range []string{"cherry", "apple", "banana"} {
		tree.Insert(hot.Key(w), w)
	}

	// Leaves are visited in key order.
	tree.ForEach(func(node hot.Node) bool {
		fmt.Println(string(node.Key()))
		return true
	})
	// Output:
	// apple
	// banana
	// cherry
}

func ExampleTree_ForEachPrefix() {
	tree := hot.New()
	for _, w := range []string{"api", "api.v1", "api.v2", "app", "zoo"} {
		tree.Insert(hot.Key(w), w)
	}

	tree.ForEachPrefix(hot.Key("api"), func(node hot.Node) bool {
		fmt.Println(string(node.Key()))
		return true
	})
	// Output:
	// api
	// api.v1
	// api.v2
}

func ExampleTree_Iterator() {
	tree := hot.New()
	tree.Insert(hot.Key("b"), 2)
	tree.Insert(hot.Key("a"), 1)

	for it := tree.Iterator(); it.HasNext(); {
		node, _ := it.Next()
		fmt.Println(string(node.Key()), node.Value())
	}
	// Output:
	// a 1
	// b 2
}

func ExampleTree_Minimum() {
	tree := hot.New()
	tree.Insert(hot.Key("banana"), 2)
	tree.Insert(hot.Key("apple"), 1)

	minimum, _ := tree.Minimum()
	maximum, _ := tree.Maximum()
	fmt.Println(minimum, maximum)
	// Output: 1 2
}
