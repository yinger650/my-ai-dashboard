package store

import "testing"

func TestMergeServiceMetadataPathAndURL(t *testing.T) {
	got := MergeServiceMetadata("{}", map[string]any{"path": "/usr/sbin/nginx", "noise": 1})
	if ParseServicePath(got) != "/usr/sbin/nginx" {
		t.Fatalf("path = %s json=%s", ParseServicePath(got), got)
	}
	got = MergeServiceMetadata(got, map[string]any{"url": "https://yinger650.com/"})
	if ParseServicePath(got) != "/usr/sbin/nginx" {
		t.Fatal("path should win over later url")
	}
	urlOnly := MergeServiceMetadata("{}", map[string]any{"url": "https://www.yinger650.com/"})
	if ParseServicePath(urlOnly) != "https://www.yinger650.com/" {
		t.Fatalf("url fallback = %s", ParseServicePath(urlOnly))
	}
	kept := MergeServiceMetadata(got, nil)
	if ParseServicePath(kept) != "/usr/sbin/nginx" {
		t.Fatal("empty metadata cleared path")
	}
	kept = MergeServiceMetadata(got, map[string]any{"path": ""})
	if ParseServicePath(kept) != "/usr/sbin/nginx" {
		t.Fatal("empty path cleared stored path")
	}
}
