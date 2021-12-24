

.PHONY: bins
bins:
	go build ./cmd/akt


.PHONY: clean
clean:
	rm -f ./akt
