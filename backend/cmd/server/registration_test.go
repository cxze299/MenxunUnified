package main

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestCanonicalRosterName(t *testing.T) {
	name, minor := canonicalRosterName(" 高亚拉（辅修） ")
	if name != "高亚拉" || !minor {
		t.Fatalf("unexpected canonical result: %q, %v", name, minor)
	}
	if got := usernameFromName("张迦勒"); got != "zhangjiale" {
		t.Fatalf("unexpected pinyin username: %s", got)
	}
}

func TestParseSimpleRoster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roster.xlsx")
	book := excelize.NewFile()
	defer book.Close()
	book.SetSheetName("Sheet1", "成员名单")
	inputRows := [][]any{
		{"小组编码", "小组名称", "成员姓名", "是否组长", "是否辅修"},
		{"truth-a", "真理 A 组", "高亚拉", "是", "否"},
		{"truth-a", "真理 A 组", "李明（辅修）", "否", "否"},
	}
	for rowIndex, row := range inputRows {
		for columnIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			if err := book.SetCellValue("成员名单", cell, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := book.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	rows, err := parseRoster(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 roster entries, got %d", len(rows))
	}
	if rows[0].GroupCode != "truth-a" || rows[0].GroupName != "真理 A 组" || !rows[0].IsLeader {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Name != "李明" || !rows[1].IsMinor {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestValidRegistrationPassword(t *testing.T) {
	for _, value := range []string{"short1", "onlyletters", "12345678"} {
		if validRegistrationPassword(value) {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
	if !validRegistrationPassword("simple123") {
		t.Fatal("expected mixed password to be accepted")
	}
}
