.PHONY: fmt clean

remirror: *.go
	go build -o remirror .

fmt:
	gofmt -w .

clean:
	rm -f remirror

