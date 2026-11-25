package git

import "testing"

const sampleDiff = `diff --git a/src/app.js b/src/app.js
index 111..222 100644
--- a/src/app.js
+++ b/src/app.js
@@ -10,0 +11,2 @@ function main() {
+  console.log("x");
+  debugger;
@@ -20 +22 @@
-old
+new
diff --git a/.env b/.env
new file mode 100644
index 0000000..333
--- /dev/null
+++ b/.env
@@ -0,0 +1 @@
+SECRET=1
diff --git a/old.txt b/new.txt
similarity index 100%
rename from old.txt
rename to new.txt
diff --git a/img.png b/img.png
index 1..2 100644
Binary files a/img.png and b/img.png differ
`

func TestParseDiff(t *testing.T) {
	files := ParseDiff(sampleDiff)
	if len(files) != 4 {
		t.Fatalf("esperava 4 arquivos, veio %d", len(files))
	}

	app := files[0]
	if app.Path != "src/app.js" || app.Status != StatusModified {
		t.Errorf("arquivo 0 inesperado: %+v", app)
	}
	if len(app.Added) != 3 || app.Removed != 1 {
		t.Errorf("contagem errada: +%d -%d", len(app.Added), app.Removed)
	}
	if app.Added[0].No != 11 || app.Added[1].No != 12 || app.Added[2].No != 22 {
		t.Errorf("números de linha errados: %+v", app.Added)
	}
	if app.Added[1].Text != "  debugger;" {
		t.Errorf("texto da linha errado: %q", app.Added[1].Text)
	}

	if files[1].Path != ".env" || files[1].Status != StatusAdded {
		t.Errorf("arquivo 1 inesperado: %+v", files[1])
	}
	if files[2].Status != StatusRenamed || files[2].Path != "new.txt" || files[2].OldPath != "old.txt" {
		t.Errorf("rename inesperado: %+v", files[2])
	}
	if !files[3].Binary {
		t.Errorf("img.png deveria ser binário")
	}
}
