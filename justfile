binary := "apple-notes-md"
out_dir := "bin"

default:
    @just --list

build:
    mkdir -p {{out_dir}}
    go build -o {{out_dir}}/{{binary}} .

install:
    go install .

lint:
    go test ./...
    go vet ./...

clean:
    rm -rf {{out_dir}}
