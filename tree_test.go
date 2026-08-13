package hot

// Ported from github.com/plar/go-adaptive-radix-tree tree_test.go.
// Node-kind expectations are adapted to HOT's node kinds (Node8/16/32 by
// sparse partial key width); HOT trees are deterministic in the key set, so
// the expectations are stable.

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTreeInitialization(t *testing.T) {
	t.Parallel()

	tree := New()
	assert.NotNil(t, tree)
}

func TestTreeInsertAndUpdate(t *testing.T) { //nolint:tparallel
	t.Parallel()

	tree := newTree()
	key := Key("key")

	tests := []struct {
		name        string
		insertKey   Key
		insertValue string
		expectedOld any
		expectedUpd bool
	}{
		{
			name:        "Initial insert",
			insertKey:   key,
			insertValue: "value",
			expectedOld: nil,
			expectedUpd: false,
		},
		{
			name:        "Update existing key",
			insertKey:   key,
			insertValue: "otherValue",
			expectedOld: "value",
			expectedUpd: true,
		},
	}

	for _, tt := range tests { //nolint:paralleltest
		t.Run(tt.name, func(t *testing.T) {
			oldValue, updated := tree.Insert(tt.insertKey, tt.insertValue)
			assert.Equal(t, tt.expectedOld, oldValue)
			assert.Equal(t, tt.expectedUpd, updated)
		})
	}

	assert.Equal(t, 1, tree.Size())

	val, found := tree.Search(key)
	assert.True(t, found)
	assert.Equal(t, "otherValue", val)
}

func TestTreeInsertSimilarPrefix(t *testing.T) {
	t.Parallel()

	tree := newTree()
	tree.Insert(Key{1}, 1)
	tree.Insert(Key{1, 1}, 11)

	val, found := tree.Search(Key{1, 1})
	assert.True(t, found)
	assert.Equal(t, 11, val)
}

func TestTreeMultipleInsertAndSearch(t *testing.T) {
	t.Parallel()

	tree := newTree()
	searchTerms := []string{"A", "a", "aa"}

	for _, term := range searchTerms {
		tree.Insert(Key(term), term)
	}

	for _, term := range searchTerms {
		val, found := tree.Search(Key(term))
		assert.True(t, found)
		assert.Equal(t, term, val)
	}
}

// HOT keeps single-byte keys in Node8 compound nodes with up to 32 entries;
// growth happens by node splits instead of ART's node kind upgrades.
func TestTreeInsertAndNodeGrowth(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		totalNodes    byte
		expectedKind  Kind
		expectedNodes int
	}{
		{5, Node8, 1},
		{17, Node8, 1},
		{32, Node8, 1},
		{33, Node8, 2},
		{49, Node8, 3},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d nodes", tc.totalNodes), func(t *testing.T) {
			t.Parallel()

			tree := newTree()
			for i := byte(0); i < tc.totalNodes; i++ {
				tree.Insert(Key{i}, i)
			}

			assert.Equal(t, int(tc.totalNodes), tree.Size())
			assert.Equal(t, tc.expectedKind, rootKind(tree))

			stats := collectStats(tree.Iterator(TraverseAll))
			assert.Equal(t, tc.expectedNodes, stats.node8Count+stats.node16Count+stats.node32Count)
		})
	}
}

func TestTreeWordsInsertMinMax(t *testing.T) {
	t.Parallel()

	tree, _ := sharedWordsTree()

	minimum := minimumLeaf(tree.root)
	assert.Equal(t, []byte("A"), minimum.value)

	maximum := maximumLeaf(tree.root)
	assert.Equal(t, []byte("zythum"), maximum.value)
}

func TestTreeUUIDsInserMinMax(t *testing.T) {
	t.Parallel()

	tree, _ := treeWithData("test/assets/uuid.txt")

	minimum := minimumLeaf(tree.root)
	assert.Equal(t, []byte("00026bda-e0ea-4cda-8245-522764e9f325"), minimum.value)

	maximum := maximumLeaf(tree.root)
	assert.Equal(t, []byte("ffffcb46-a92e-4822-82af-a7190f9c1ec5"), maximum.value)
}

func TestTreeInsertAndDeleteOperations(t *testing.T) { //nolint:funlen,cyclop
	t.Parallel()

	var tests = []*testDataset{
		{
			name:         "Insert 1 Delete 1",
			insertItems:  []string{"test"},
			deleteItems:  []string{"test"},
			expectedSize: 0,
			expectedRoot: nil,
			deleteStatus: true,
		},
		{
			name:         "Insert 2 Delete 1",
			insertItems:  []string{"test1", "test2"},
			deleteItems:  []string{"test2"},
			expectedSize: 1,
			expectedRoot: Leaf,
			deleteStatus: true,
		},
		{
			name:         "Insert 2 Delete 2",
			insertItems:  []string{"test1", "test2"},
			deleteItems:  []string{"test1", "test2"},
			expectedSize: 0,
			expectedRoot: nil,
			deleteStatus: true,
		},
		{
			name:         "Insert 5 Delete 1",
			insertItems:  []string{"1", "2", "3", "4", "5"},
			deleteItems:  []string{"1"},
			expectedSize: 4,
			expectedRoot: Node8,
			deleteStatus: true,
		},
		{
			name:         "Insert 5 Try to delete 1 wrong",
			insertItems:  []string{"1", "2", "3", "4", "5"},
			deleteItems:  []string{"123"},
			expectedSize: 5,
			expectedRoot: Node8,
			deleteStatus: false},
		{
			name:         "Insert 5 Delete 5",
			insertItems:  []string{"1", "2", "3", "4", "5"},
			deleteItems:  []string{"1", "2", "3", "4", "5"},
			expectedSize: 0,
			expectedRoot: nil,
			deleteStatus: true,
		},
		{
			name:         "Insert 17 Delete 1",
			insertItems:  []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17"},
			deleteItems:  []string{"2"},
			expectedSize: 16,
			expectedRoot: Node8,
			deleteStatus: true,
		},
		{
			name:         "Insert 17 Delete 17",
			insertItems:  []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17"},
			deleteItems:  []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15", "16", "17"},
			expectedSize: 0,
			expectedRoot: nil,
			deleteStatus: true,
		},
		{
			name: "Insert 49 Delete 0",
			insertItems: testDatasetBuilder(func(_ *testDataset, tree *tree) {
				for i := range byte(49) {
					tree.Insert(Key{i}, []byte{i})
				}
			}),
			deleteItems:  []byte{byte(123)},
			expectedSize: 49,
			expectedRoot: Node8,
			deleteStatus: false},
		{
			name: "Insert 49 Delete 1",
			insertItems: testDatasetBuilder(func(_ *testDataset, tree *tree) {
				for i := range byte(49) {
					tree.Insert(Key{i}, []byte{i})
				}
			}),
			deleteItems:  []byte{byte(2)},
			expectedSize: 48,
			expectedRoot: Node8,
			deleteStatus: true,
		},
		{
			name: "Insert 49 Delete 49",
			insertItems: testDatasetBuilder(func(_ *testDataset, tree *tree) {
				for i := range byte(49) {
					tree.Insert(Key{i}, []byte{i})
				}
			}),
			deleteItems: testDatasetBuilder(func(data *testDataset, tree *tree) {
				for i := range byte(49) {
					term := []byte{i}
					val, deleted := tree.Delete(term)
					assert.True(t, deleted, data.name)
					assert.Equal(t, term, val)
				}
			}),
			expectedSize: 0,
			expectedRoot: nil,
			deleteStatus: true,
		},
		{
			name: "Insert 256 Delete 1",
			insertItems: testDatasetBuilder(func(_ *testDataset, tree *tree) {
				for i := range 256 {
					term := bytes.NewBuffer([]byte{})
					term.WriteByte(byte(i))
					tree.Insert(term.Bytes(), term.Bytes())
				}
			}),
			deleteItems:  []byte{2},
			expectedSize: 255,
			expectedRoot: Node8,
			deleteStatus: true,
		},
		{
			name: "Insert 256 Delete 256",
			insertItems: testDatasetBuilder(func(_ *testDataset, tree *tree) {
				for i := range 256 {
					term := strconv.Itoa(i)
					tree.Insert(Key(term), term)
				}
			}),
			deleteItems: testDatasetBuilder(func(data *testDataset, tree *tree) {
				for i := range 256 {
					term := strconv.Itoa(i)
					val, deleted := tree.Delete(Key(term))
					assert.Equal(t, data.deleteStatus, deleted)
					assert.Equal(t, term, val)
				}
			}),
			expectedSize: 0,
			expectedRoot: nil,
			deleteStatus: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tree := newTree()
			tt.build(t, tree)
			tt.process(t, tree)
			tt.assert(t, tree)
		})
	}
}

func TestDeleteNonexistentPrefix(t *testing.T) {
	t.Parallel()

	tree := newTree()
	tree.Insert(Key("keyb::"), "0")
	tree.Insert(Key("keyb::1"), "1")
	tree.Insert(Key("keyb::2"), "2")

	v, deleted := tree.Delete(Key("keyb:"))
	assert.Nil(t, v)
	assert.False(t, deleted)
}

// Inserting a single value into the tree and removing it should result in a nil tree root.
func TestInsertAndDeleteOne(t *testing.T) {
	t.Parallel()

	tree := newTree()
	tree.Insert(Key("test"), "data")
	v, deleted := tree.Delete(Key("test"))
	assert.True(t, deleted)
	assert.Equal(t, "data", v)
	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestInsertTwoAndDeleteOne(t *testing.T) {
	t.Parallel()

	tree := newTree()
	tree.Insert(Key("2"), 2)
	tree.Insert(Key("1"), 1)

	_, found := tree.Search(Key("2"))
	assert.True(t, found)
	_, found = tree.Search(Key("1"))
	assert.True(t, found)

	val, deleted := tree.Delete(Key("2"))
	assert.True(t, deleted)
	assert.Equal(t, 2, val)

	_, found = tree.Search(Key("2"))
	assert.False(t, found)

	assert.Equal(t, 1, tree.size)
	assert.Equal(t, Leaf, rootKind(tree))
}

func TestInsertTwoAndDeleteTwo(t *testing.T) {
	t.Parallel()

	tree := newTree()
	tree.Insert(Key("2"), 2)
	tree.Insert(Key("1"), 1)

	_, found := tree.Search(Key("2"))
	assert.True(t, found)
	_, found = tree.Search(Key("1"))
	assert.True(t, found)

	v, deleted := tree.Delete(Key("2"))
	assert.True(t, deleted)
	assert.Equal(t, 2, v)

	v, deleted = tree.Delete(Key("1"))
	assert.True(t, deleted)
	assert.Equal(t, 1, v)

	_, found = tree.Search(Key("2"))
	assert.False(t, found)

	_, found = tree.Search(Key("1"))
	assert.False(t, found)

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestTreeInsertSearchDeleteWords(t *testing.T) {
	t.Parallel()

	tree, words := treeWithData("test/assets/words.txt")
	for _, w := range words {
		v, found := tree.Search(w)
		assert.Equal(t, w, v, string(w))
		assert.True(t, found)
	}

	for _, w := range words {
		v, deleted := tree.Delete(w)
		assert.True(t, deleted)
		assert.Equal(t, w, v)
	}

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestTreeInsertSearchDeleteHSKWords(t *testing.T) {
	t.Parallel()

	tree, words := treeWithData("test/assets/hsk_words.txt")
	for _, w := range words {
		v, found := tree.Search(w)
		assert.Equal(t, w, v, string(w))
		assert.True(t, found)
	}

	for _, w := range words {
		v, deleted := tree.Delete(w)
		assert.True(t, deleted)
		assert.Equal(t, w, v)
	}

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestTreeInsertSearchDeleteUUIDs(t *testing.T) {
	t.Parallel()

	tree, words := treeWithData("test/assets/uuid.txt")
	uuids := words
	for _, w := range words {
		v, found := tree.Search(w)
		assert.Equal(t, w, v, string(w))
		assert.True(t, found)
	}

	for _, w := range uuids {
		v, deleted := tree.Delete(w)
		assert.True(t, deleted)
		assert.Equal(t, w, v)
	}

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestInsertDeleteWithZeroChild(t *testing.T) {
	t.Parallel()

	keys := []Key{
		Key("test/a1"),
		Key("test/a2"),
		Key("test/a3"),
		Key("test/a4"),
		Key("test/a"), // proper prefix of the other keys
	}

	tree := newTree()
	for _, w := range keys {
		tree.Insert(w, w)
	}

	// "test/a" must be the smallest leaf and searchable.
	min := minimumLeaf(tree.root)
	assert.Equal(t, Leaf, min.Kind())
	assert.Equal(t, Key("test/a"), min.Key())

	v, found := tree.Search(Key("test/a"))
	assert.True(t, found)
	assert.Equal(t, Value(Key("test/a")), v)

	for _, w := range keys {
		v, deleted := tree.Delete(w)
		assert.True(t, deleted)
		assert.Equal(t, w, v)
	}

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestTreeAPI(t *testing.T) { //nolint:funlen
	t.Parallel()

	// test empty tree
	tree := New()
	assert.NotNil(t, tree)
	assert.Equal(t, 0, tree.Size())

	oldValue, deleted := tree.Delete(Key("non existent key"))
	assert.Nil(t, oldValue)
	assert.False(t, deleted)

	minValue, found := tree.Minimum()
	assert.Nil(t, minValue)
	assert.False(t, found)

	maxValue, found := tree.Maximum()
	assert.Nil(t, maxValue)
	assert.False(t, found)

	value, found := tree.Search(Key("non existent key"))
	assert.Nil(t, value)
	assert.False(t, found)

	it := tree.Iterator(TraverseAll)
	assert.NotNil(t, it)
	assert.False(t, it.HasNext())
	_, err := it.Next()
	require.Error(t, err)
	assert.Equal(t, ErrNoMoreNodes, err)

	tree.ForEach(func(Node) bool {
		t.Fatalf("Should not be called on an empty tree")

		return true
	})

	// Insert Key-Value Pairs
	tree = New()
	oldValue, updated := tree.Insert(Key("Hi, I'm Key"), "Nice to meet you, I'm Value")
	assert.Nil(t, oldValue)
	assert.False(t, updated)
	assert.Equal(t, 1, tree.Size())

	value, found = tree.Search(Key("Hi, I'm Key"))
	assert.True(t, found)
	assert.Equal(t, "Nice to meet you, I'm Value", value)

	oldValue, updated = tree.Insert(Key("Hi, I'm Key"), "Ha-ha, Again? Go away!")
	assert.Equal(t, "Nice to meet you, I'm Value", oldValue)
	assert.True(t, updated)
	assert.Equal(t, 1, tree.Size())

	value, found = tree.Search(Key("Hi, I'm Key"))
	assert.True(t, found)
	assert.Equal(t, "Ha-ha, Again? Go away!", value)

	tree.ForEach(func(node Node) bool {
		assert.Equal(t, Key("Hi, I'm Key"), node.Key())
		assert.Equal(t, "Ha-ha, Again? Go away!", node.Value())

		return true
	})

	it = tree.Iterator(TraverseAll)
	assert.NotNil(t, it)
	assert.True(t, it.HasNext())

	next, err := it.Next()
	require.NoError(t, err)
	assert.NotNil(t, value)
	assert.Equal(t, "Ha-ha, Again? Go away!", next.Value())

	assert.False(t, it.HasNext())
	next, err = it.Next()
	assert.Nil(t, next)
	require.Error(t, err)

	oldValue, updated = tree.Insert(Key("Hi, I'm Value"), "Surprise!")
	assert.Nil(t, oldValue)
	assert.False(t, updated)

	tree.Insert(Key("Now I know..."), "What?")
	tree.Insert(Key("ABC"), "ABC")
	tree.Insert(Key("DEF"), "DEF")
	tree.Insert(Key("XYZ"), "XYZ")

	minValue, found = tree.Minimum()
	assert.Equal(t, "ABC", minValue)
	assert.True(t, found)

	maxValue, found = tree.Maximum()
	assert.Equal(t, "XYZ", maxValue)
	assert.True(t, found)
}

func TestTreeInsertAndSearchKeyWithNull(t *testing.T) {
	t.Parallel()

	tree := newTree()
	terms := []string{"ab\x00", "ab", "ad", "ac"}

	for _, term := range terms {
		tree.Insert(Key(term), term)
	}

	for _, term := range terms {
		v, found := tree.Search(Key(term))
		assert.True(t, found)
		assert.Equal(t, term, v)
	}

	expected := []string{"ab", "ab\x00", "ac", "ad"}
	traversal := []string{}

	tree.ForEach(func(node Node) bool {
		traversal = append(traversal, string(node.Key()))

		return true
	}, TraverseLeaf)
	assert.Equal(t, expected, traversal)

	traversal = []string{}

	it := tree.Iterator(TraverseLeaf)
	for it.HasNext() {
		leaf, _ := it.Next()
		traversal = append(traversal, string(leaf.Key()))
	}

	assert.Equal(t, expected, traversal)
}

func TestNodesWithNullKeys4(t *testing.T) {
	t.Parallel()

	tree := newTree()

	terms := []string{"aa", "aa\x00", "aac", "aab\x00"}
	for _, term := range terms {
		tree.Insert(Key(term), term)
	}

	for _, term := range terms {
		v, found := tree.Search(Key(term))
		assert.True(t, found)
		assert.Equal(t, term, v)
	}
}

func TestNodesWithNullKeys16(t *testing.T) { //nolint:funlen
	t.Parallel()

	tree := newTree()
	terms := []string{ // shuffled, no order
		"aad\x00",
		"aam\x00",
		"aae\x00",
		"aal\x00",
		"aab",
		"aaq\x00",
		"aa",
		"aax\x00",
		"aaf\x00",
		"aag\x00",
		"aaz\x00",
		"aav\x00",
		"aaj\x00",
		"aak\x00",
		"aah\x00",
		"aac\x00",
		"aa\x00"}

	for _, term := range terms {
		tree.Insert(Key(term), term)
	}

	for _, term := range terms {
		v, found := tree.Search(Key(term))
		assert.True(t, found)
		assert.Equal(t, term, v)
	}

	v, found := tree.Minimum()
	assert.Equal(t, "aa", v)
	assert.True(t, found)

	expected := []string{
		"aa",
		"aa\x00",
		"aab",
		"aac\x00",
		"aad\x00",
		"aae\x00",
		"aaf\x00",
		"aag\x00",
		"aah\x00",
		"aaj\x00",
		"aak\x00",
		"aal\x00",
		"aam\x00",
		"aaq\x00",
		"aav\x00",
		"aax\x00",
		"aaz\x00",
	}
	traversal := []string{}

	tree.ForEach(func(node Node) bool {
		traversal = append(traversal, string(node.Key()))

		return true
	}, TraverseLeaf)
	assert.Equal(t, expected, traversal)
}

func TestNodesWithNullKeys48(t *testing.T) { //nolint:funlen
	t.Parallel()

	tree := newTree()
	terms := []string{
		"aab",
		"aa\x00",
		"aac\x00",
		"aad\x00",
		"aae\x00",
		"aaf\x00",
		"aag\x00",
		"aah\x00",
		"aaj\x00",
		"aak\x00",
		"aal\x00",
		"aaz\x00",
		"aax\x00",
		"aav\x00",
		"aam\x00",
		"aaq\x00",
		"aa",
	}

	for _, term := range terms {
		tree.Insert(Key(term), term)
	}

	for _, term := range terms {
		v, found := tree.Search(Key(term))
		assert.True(t, found)
		assert.Equal(t, term, v)
	}

	// find minimum term with null prefix
	v, found := tree.Minimum()
	assert.True(t, found)
	assert.Equal(t, "aa", v)

	// traverse include term with null prefix
	termsCopy := make([]string, len(terms))
	copy(termsCopy, terms)

	traversal := []string{}

	tree.ForEach(func(node Node) bool {
		s, _ := node.Value().(string)
		traversal = append(traversal, s)

		return true
	}, TraverseLeaf)
	slices.Sort(termsCopy) // traversal should be in sorted order
	assert.Equal(t, termsCopy, traversal)

	// delete all terms
	for _, term := range terms {
		v, deleted := tree.Delete(Key(term))
		assert.True(t, deleted)
		assert.Equal(t, term, v)
	}

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestNodesWithNullKeys256(t *testing.T) { //nolint:funlen
	t.Parallel()

	tree := newTree()
	terms := []string{"b"}

	// build list of terms which will use several compound nodes
	for i0 := 0; i0 <= 260; i0++ {
		var term string
		if i0 < 130 {
			term = string([]byte{'a', byte(i0)})
		} else {
			term = string([]byte{'b', byte(i0)})
		}

		terms = append(terms, term)
	}

	for _, term := range terms {
		_, updated := tree.Insert(Key(term), term)
		assert.False(t, updated)
	}

	// insert a term with null prefix
	term := "a"
	_, updated := tree.Insert(Key(term), term)
	assert.False(t, updated)

	for _, term := range terms {
		v, found := tree.Search(Key(term))
		assert.True(t, found)
		assert.Equal(t, term, v)
	}

	// find minimum term with null prefix
	v, found := tree.Minimum()
	assert.True(t, found)
	assert.Equal(t, term, v)

	// traverse include term with null prefix
	termsCopy := make([]string, len(terms))
	copy(termsCopy, terms)
	termsCopy = append(termsCopy, term) //nolint:makezero
	traversal := []string{}

	tree.ForEach(func(node Node) bool {
		s, _ := node.Value().(string)
		traversal = append(traversal, s)

		return true
	}, TraverseLeaf)
	slices.Sort(termsCopy)
	assert.Equal(t, termsCopy, traversal)

	// delete a term with null prefix
	v, deleted := tree.Delete(Key(term))
	assert.True(t, deleted)
	assert.Equal(t, term, v)

	for _, term := range terms {
		v, deleted := tree.Delete(Key(term))
		assert.True(t, deleted)
		assert.Equal(t, term, v)
	}

	assert.Equal(t, 0, tree.size)
	assert.Nil(t, tree.root)
}

func TestTreeInsertAndSearchKeyWithUnicodeAccentChar(t *testing.T) {
	t.Parallel()

	tree := newTree()
	smallA := "a"
	accent := []byte{smallA[0], 0x00, 0x60} // ‘a' followed by unicode accent character.
	tree.Insert([]byte(smallA), smallA)
	tree.Insert(accent, string(accent))

	v, found := tree.Search([]byte("a"))
	assert.True(t, found)
	assert.Equal(t, smallA, v)

	v, found = tree.Search(accent)
	assert.True(t, found)
	assert.Equal(t, string(accent), v)
}

func TestTreeInsertNilKeyTwice(t *testing.T) {
	t.Parallel()

	tree := newTree()

	kk := Key("key")
	kv := "kk-value"
	old, updated := tree.Insert(kk, kv)
	assert.Nil(t, old)
	assert.False(t, updated)

	v, found := tree.Search(kk)
	assert.Equal(t, kv, v)
	assert.True(t, found)

	knil := Key(nil)
	knilv0 := "knil-value-0"
	old, updated = tree.Insert(knil, knilv0)
	assert.Nil(t, old)
	assert.False(t, updated)

	v, found = tree.Search(knil)
	assert.Equal(t, knilv0, v)
	assert.True(t, found)

	knilv1 := "knil-value-1"
	old, updated = tree.Insert(knil, knilv1)
	assert.Equal(t, knilv0, old)
	assert.True(t, updated)

	v, found = tree.Search(knil)
	assert.Equal(t, knilv1, v)
	assert.True(t, found)
}
