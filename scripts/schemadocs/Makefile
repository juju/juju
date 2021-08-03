output.html: ../../apiserver/facades/schema.json
	go run ./schemadocs.go ../../apiserver/facades/schema.json > output.html

open: output.html
	xdg-open output.html >/dev/null 2>&1

clean:
	rm output.html
