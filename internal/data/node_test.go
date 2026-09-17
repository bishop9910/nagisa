package data

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"nagisa/internal/biz"

	"github.com/google/uuid"
)

// 回收站要保住层级：删掉的文件夹是顶层的一项，它里面的内容得走进该文件夹才看得到，
// 不能和被删的文件夹平铺在同一级。
func TestListNodesTrashRoots(t *testing.T) {
	ctx := context.Background()
	db, _, err := openSQLite(DataOptions{Source: filepath.Join(t.TempDir(), "trash.db"), ForeignKeys: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Schema.Create(ctx); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}

	repo := NewNodeRepo(&Data{db: db})
	owner := uuid.Must(uuid.NewV7())

	docsID := uuid.Must(uuid.NewV7())
	docsPath := biz.ChildPath("/", docsID)
	docs := createTestNode(t, ctx, repo, &biz.Node{
		ID: docsID, ParentID: uuid.Nil, Path: docsPath, Depth: 0,
		Name: "docs", NameLower: "docs", Kind: biz.NodeKindFolder, OwnerID: owner, Status: biz.NodeStatusActive,
	})
	innerID := uuid.Must(uuid.NewV7())
	createTestNode(t, ctx, repo, &biz.Node{
		ID: innerID, ParentID: docsID, Path: biz.ChildPath(docsPath, innerID), Depth: 1,
		Name: "contract.pdf", NameLower: "contract.pdf", Kind: biz.NodeKindFile, OwnerID: owner, Status: biz.NodeStatusActive,
	})
	looseID := uuid.Must(uuid.NewV7())
	createTestNode(t, ctx, repo, &biz.Node{
		ID: looseID, ParentID: uuid.Nil, Path: biz.ChildPath("/", looseID), Depth: 0,
		Name: "loose.png", NameLower: "loose.png", Kind: biz.NodeKindFile, OwnerID: owner, Status: biz.NodeStatusActive,
	})

	trashTestNode(t, ctx, repo, docs)
	loose, err := repo.FindNodeByID(ctx, looseID)
	if err != nil {
		t.Fatalf("load loose node: %v", err)
	}
	trashTestNode(t, ctx, repo, loose)

	status := biz.NodeStatusTrashed

	// 顶层只有被删的那两项，文件夹里面的文件不在这一级。
	roots := listTestNames(t, ctx, repo, biz.NodeQuery{Status: &status, IncludeTrashed: true, TrashRoots: true})
	if got := roots; len(got) != 2 || got[0] != "docs" || got[1] != "loose.png" {
		t.Errorf("trash roots = %v, want [docs loose.png]", got)
	}

	// 走进被删的文件夹，才看到它里面的条目。
	children := listTestNames(t, ctx, repo, biz.NodeQuery{Status: &status, IncludeTrashed: true, ParentID: &docsID})
	if got := children; len(got) != 1 || got[0] != "contract.pdf" {
		t.Errorf("children of the trashed folder = %v, want [contract.pdf]", got)
	}

	// 分页依赖的总数必须和列表口径一致。
	total, err := repo.CountNodes(ctx, biz.NodeQuery{Status: &status, IncludeTrashed: true, TrashRoots: true})
	if err != nil {
		t.Fatalf("count trash roots: %v", err)
	}
	if total != 2 {
		t.Errorf("trash root count = %d, want 2", total)
	}

	// 不带 TrashRoots 时仍是整棵子树（清空回收站等要看到全部行）。
	all := listTestNames(t, ctx, repo, biz.NodeQuery{Status: &status, IncludeTrashed: true})
	if len(all) != 3 {
		t.Errorf("full trashed listing = %v, want 3 entries", all)
	}
}

func createTestNode(t *testing.T, ctx context.Context, repo biz.NodeRepo, n *biz.Node) *biz.Node {
	t.Helper()
	if n.ID == uuid.Nil {
		n.ID = uuid.Must(uuid.NewV7())
	}
	if n.Path == "" {
		n.Path = biz.ChildPath("/", n.ID)
	}
	created, err := repo.CreateNode(ctx, n)
	if err != nil {
		t.Fatalf("create node %q: %v", n.Name, err)
	}
	return created
}

func trashTestNode(t *testing.T, ctx context.Context, repo biz.NodeRepo, n *biz.Node) {
	t.Helper()
	if _, err := repo.SetSubtreeStatus(ctx, n, biz.NodeStatusTrashed, nil); err != nil {
		t.Fatalf("trash node %q: %v", n.Name, err)
	}
}

func listTestNames(t *testing.T, ctx context.Context, repo biz.NodeRepo, q biz.NodeQuery) []string {
	t.Helper()
	nodes, err := repo.ListNodes(ctx, q)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	sort.Strings(names)
	return names
}
