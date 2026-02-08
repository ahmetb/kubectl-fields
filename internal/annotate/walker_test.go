package annotate

import (
	"testing"
	"time"

	"github.com/rewanthtammana/kubectl-fields/internal/managed"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
)

// Helper to build a ScalarNode.
func scalarNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
}

// Helper to build a MappingNode from key-value pairs.
func mappingNode(pairs ...*yaml.Node) *yaml.Node {
	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: pairs,
	}
}

// Helper to build an empty MappingNode (FieldsV1 leaf marker).
func emptyMapping() *yaml.Node {
	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: []*yaml.Node{},
	}
}

func TestFindMappingField(t *testing.T) {
	t.Run("finds existing key", func(t *testing.T) {
		m := mappingNode(
			scalarNode("replicas"), scalarNode("3"),
			scalarNode("image"), scalarNode("nginx"),
		)
		k, v := findMappingField(m, "replicas")
		assert.NotNil(t, k)
		assert.NotNil(t, v)
		assert.Equal(t, "replicas", k.Value)
		assert.Equal(t, "3", v.Value)
	})

	t.Run("returns nil for missing key", func(t *testing.T) {
		m := mappingNode(
			scalarNode("replicas"), scalarNode("3"),
		)
		k, v := findMappingField(m, "image")
		assert.Nil(t, k)
		assert.Nil(t, v)
	})

	t.Run("handles non-mapping node", func(t *testing.T) {
		s := scalarNode("hello")
		k, v := findMappingField(s, "anything")
		assert.Nil(t, k)
		assert.Nil(t, v)
	})

	t.Run("handles nil node", func(t *testing.T) {
		k, v := findMappingField(nil, "anything")
		assert.Nil(t, k)
		assert.Nil(t, v)
	})
}

func TestIsLeaf(t *testing.T) {
	t.Run("empty mapping is leaf", func(t *testing.T) {
		assert.True(t, isLeaf(emptyMapping()))
	})

	t.Run("non-empty mapping is not leaf", func(t *testing.T) {
		m := mappingNode(
			scalarNode("f:name"), emptyMapping(),
		)
		assert.False(t, isLeaf(m))
	})

	t.Run("scalar is not leaf", func(t *testing.T) {
		assert.False(t, isLeaf(scalarNode("hello")))
	})
}

func TestWalkFieldsV1_SimpleScalarFields(t *testing.T) {
	// YAML: replicas: 3, image: nginx
	yamlRoot := mappingNode(
		scalarNode("replicas"), scalarNode("3"),
		scalarNode("image"), scalarNode("nginx"),
	)

	// FieldsV1: {f:replicas: {}, f:image: {}}
	fieldsV1 := mappingNode(
		scalarNode("f:replicas"), emptyMapping(),
		scalarNode("f:image"), emptyMapping(),
	)

	entry := managed.ManagedFieldsEntry{
		Manager: "kubectl-client-side-apply",
		Time:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	targets := make(map[*yaml.Node]AnnotationTarget)
	walkFieldsV1(yamlRoot, nil, fieldsV1, entry, targets)

	assert.Len(t, targets, 2)

	// Find the replicas value node (Content[1])
	replicasVal := yamlRoot.Content[1]
	target, ok := targets[replicasVal]
	assert.True(t, ok, "replicas value node should be in targets")
	assert.Equal(t, "kubectl-client-side-apply", target.Info.Manager)
	assert.Equal(t, "replicas", target.KeyNode.Value)
	assert.Equal(t, "3", target.ValueNode.Value)

	// Find the image value node (Content[3])
	imageVal := yamlRoot.Content[3]
	target, ok = targets[imageVal]
	assert.True(t, ok, "image value node should be in targets")
	assert.Equal(t, "kubectl-client-side-apply", target.Info.Manager)
	assert.Equal(t, "image", target.KeyNode.Value)
	assert.Equal(t, "nginx", target.ValueNode.Value)
}

func TestWalkFieldsV1_NestedFields(t *testing.T) {
	// YAML: metadata: { labels: { app: nginx } }
	appKey := scalarNode("app")
	appVal := scalarNode("nginx")
	labelsMapping := mappingNode(appKey, appVal)
	labelsKey := scalarNode("labels")
	metadataMapping := mappingNode(labelsKey, labelsMapping)
	yamlRoot := mappingNode(scalarNode("metadata"), metadataMapping)

	// FieldsV1: {f:metadata: {f:labels: {.: {}, f:app: {}}}}
	fieldsV1 := mappingNode(
		scalarNode("f:metadata"), mappingNode(
			scalarNode("f:labels"), mappingNode(
				scalarNode("."), emptyMapping(),
				scalarNode("f:app"), emptyMapping(),
			),
		),
	)

	entry := managed.ManagedFieldsEntry{
		Manager: "kubectl-edit",
		Time:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	targets := make(map[*yaml.Node]AnnotationTarget)
	walkFieldsV1(yamlRoot, nil, fieldsV1, entry, targets)

	// Dot target on labels mapping: KeyNode = labelsKey, ValueNode = labelsMapping
	dotTarget, ok := targets[labelsMapping]
	assert.True(t, ok, "labels mapping should have dot target")
	assert.Equal(t, labelsKey, dotTarget.KeyNode, "dot target KeyNode should be labels key")
	assert.Equal(t, labelsMapping, dotTarget.ValueNode, "dot target ValueNode should be labels mapping")
	assert.Equal(t, "kubectl-edit", dotTarget.Info.Manager)

	// Field target on app: KeyNode = appKey, ValueNode = appVal
	appTarget, ok := targets[appVal]
	assert.True(t, ok, "app value node should have target")
	assert.Equal(t, appKey, appTarget.KeyNode)
	assert.Equal(t, appVal, appTarget.ValueNode)
	assert.Equal(t, "kubectl-edit", appTarget.Info.Manager)
}

func TestWalkFieldsV1_LeafContainerField(t *testing.T) {
	// YAML: selector: { matchLabels: { app: nginx } }
	selectorMapping := mappingNode(
		scalarNode("matchLabels"), mappingNode(
			scalarNode("app"), scalarNode("nginx"),
		),
	)
	selectorKey := scalarNode("selector")
	yamlRoot := mappingNode(selectorKey, selectorMapping)

	// FieldsV1: {f:selector: {}}  -- leaf (empty mapping), so annotate selector, don't recurse
	fieldsV1 := mappingNode(
		scalarNode("f:selector"), emptyMapping(),
	)

	entry := managed.ManagedFieldsEntry{
		Manager: "kubectl-apply",
		Time:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	targets := make(map[*yaml.Node]AnnotationTarget)
	walkFieldsV1(yamlRoot, nil, fieldsV1, entry, targets)

	// selector should be annotated as a leaf
	target, ok := targets[selectorMapping]
	assert.True(t, ok, "selector value node should be in targets as a leaf")
	assert.Equal(t, selectorKey, target.KeyNode)
	assert.Equal(t, selectorMapping, target.ValueNode)
	assert.Equal(t, "kubectl-apply", target.Info.Manager)

	// Should NOT have any targets for matchLabels or app (no recursion)
	assert.Len(t, targets, 1, "only selector should be targeted, not its children")
}

func TestWalkFieldsV1_UnmanagedFieldsIgnored(t *testing.T) {
	// YAML has three fields but FieldsV1 only owns one
	yamlRoot := mappingNode(
		scalarNode("replicas"), scalarNode("3"),
		scalarNode("image"), scalarNode("nginx"),
		scalarNode("ports"), scalarNode("80"),
	)

	// FieldsV1 only owns replicas
	fieldsV1 := mappingNode(
		scalarNode("f:replicas"), emptyMapping(),
	)

	entry := managed.ManagedFieldsEntry{
		Manager: "kubectl-apply",
		Time:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	targets := make(map[*yaml.Node]AnnotationTarget)
	walkFieldsV1(yamlRoot, nil, fieldsV1, entry, targets)

	assert.Len(t, targets, 1, "only managed fields should have targets")

	// replicas should be in targets
	_, ok := targets[yamlRoot.Content[1]]
	assert.True(t, ok, "replicas should be targeted")

	// image and ports should NOT be in targets
	_, ok = targets[yamlRoot.Content[3]]
	assert.False(t, ok, "image should not be targeted")
	_, ok = targets[yamlRoot.Content[5]]
	assert.False(t, ok, "ports should not be targeted")
}

func TestWalkFieldsV1_KAndVPrefixesSkipped(t *testing.T) {
	// Ensure k: and v: prefixes are skipped without error
	yamlRoot := mappingNode(
		scalarNode("name"), scalarNode("test"),
	)

	fieldsV1 := mappingNode(
		scalarNode("f:name"), emptyMapping(),
		scalarNode("k:{\"name\":\"test\"}"), emptyMapping(),
		scalarNode("v:something"), emptyMapping(),
	)

	entry := managed.ManagedFieldsEntry{
		Manager: "test-manager",
		Time:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	targets := make(map[*yaml.Node]AnnotationTarget)
	walkFieldsV1(yamlRoot, nil, fieldsV1, entry, targets)

	// Only f:name should be in targets; k: and v: should be skipped
	assert.Len(t, targets, 1)
	_, ok := targets[yamlRoot.Content[1]]
	assert.True(t, ok, "name should be targeted")
}
