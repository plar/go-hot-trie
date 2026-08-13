package hot

import (
	"bufio"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// testDataset defines a dataset for testing the tree.
type testDataset struct {
	name         string
	insertItems  any
	deleteItems  any
	expectedSize int
	expectedRoot any
	deleteStatus bool
}

type testDatasetBuilder func(data *testDataset, tree *tree)

func (ds *testDataset) build(_ *testing.T, tree *tree) {
	switch insertData := ds.insertItems.(type) {
	case []string:
		for _, term := range insertData {
			tree.Insert(Key(term), term)
		}
	case testDatasetBuilder:
		insertData(ds, tree)
	}
}

func (ds *testDataset) process(t *testing.T, tree *tree) {
	t.Helper()

	switch deleteData := ds.deleteItems.(type) {
	case []string:
		ds.processAsStrings(t, tree, deleteData)
	case []byte:
		ds.processAsBytes(t, tree, deleteData)
	case testDatasetBuilder:
		deleteData(ds, tree)
	}
}

func (ds *testDataset) processAsStrings(t *testing.T, tree *tree, stringData []string) {
	t.Helper()

	for _, strVal := range stringData {
		ds.processSingleItem(t, tree, Key(strVal), strVal)
	}
}

func (ds *testDataset) processAsBytes(t *testing.T, tree *tree, bytesData []byte) {
	t.Helper()

	for _, byteVal := range bytesData {
		ds.processSingleItem(t, tree, Key{byteVal}, []byte{byteVal})
	}
}

func (ds *testDataset) processSingleItem(t *testing.T, tree *tree, key Key, expectedVal any) {
	t.Helper()

	val, deleted := tree.Delete(key)
	assert.Equal(t, ds.deleteStatus, deleted, ds.name)

	if deleted {
		assert.Equal(t, expectedVal, val, ds.name)
	}

	_, found := tree.Search(key)
	assert.False(t, found, ds.name)
}

func (ds *testDataset) assert(t *testing.T, tree *tree) {
	t.Helper()
	assert.Equal(t, ds.expectedSize, tree.size, ds.name)

	switch root := ds.expectedRoot.(type) {
	case Kind:
		assert.Equal(t, root, rootKind(tree), ds.name)
	case nil:
		assert.Nil(t, tree.root, ds.name)
	}
}

// rootKind returns the Kind of the root node, or -1 for an empty tree.
func rootKind(t *tree) Kind {
	if t.root == nil {
		return Kind(-1)
	}
	return asNode(t.root).Kind()
}

// treeStats defines the statistics of the tree. HOT compound nodes are
// classified by their sparse partial key width.
type treeStats struct {
	leafCount   int
	node8Count  int
	node16Count int
	node32Count int
}

// processStats processes the node statistics.
func (stats *treeStats) processStats(node Node) bool {
	switch node.Kind() {
	case Node8:
		stats.node8Count++
	case Node16:
		stats.node16Count++
	case Node32:
		stats.node32Count++
	case Leaf:
		stats.leafCount++
	}

	return true
}

// iterateWithCallback iterates the tree with the given callback.
func iterateWithCallback(it Iterator, cb func(node Node) bool) {
	for it.HasNext() {
		node, _ := it.Next()
		if !cb(node) {
			break
		}
	}
}

// collectStats collects the statistics of the tree.
func collectStats(it Iterator) treeStats {
	var stats treeStats

	iterateWithCallback(it, stats.processStats)

	return stats
}

// loadTestFile loads the test file from the given path.
func loadTestFile(path string) [][]byte {
	var words [][]byte

	file, err := os.Open(path)
	if err != nil {
		panic("Couldn't open " + path)
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReader(file)

	for {
		if line, err := reader.ReadBytes(byte('\n')); err != nil {
			break
		} else if len(line) > 0 {
			words = append(words, line[:len(line)-1])
		}
	}

	return words
}

var (
	wordsOnce   sync.Once
	sharedWords *tree
	sharedData  [][]byte
)

// sharedWordsTree returns a words tree shared by read-only tests. Callers
// must not mutate it.
func sharedWordsTree() (*tree, [][]byte) {
	wordsOnce.Do(func() {
		sharedWords, sharedData = treeWithData("test/assets/words.txt")
	})
	return sharedWords, sharedData
}

// treeWithData creates a tree with the data from the given file.
func treeWithData(filePath string) (*tree, [][]byte) {
	tree := newTree()

	data := loadTestFile(filePath)
	for _, item := range data {
		tree.Insert(item, item)
	}

	return tree, data
}
