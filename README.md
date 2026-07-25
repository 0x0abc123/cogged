# Cogged

A lightweight, minimal framework for web/mobile application backends.

- written in Golang
- cross-platform
- simple, minimal API
- provides a multi-user environment
- out-of-the-box security controls for authentication and authorisation
- backed by a graph database (Dgraph)
- provides a generic data schema using Directed Cyclic Graphs (DCGs) to structure data and define access controls and sharing

## Documentation

More detailed documentation in relation to the above points and that explains how to use this framework to build applications is located in the [about page](./docs/about.md) in this repo.

## Requirements

- Golang 1.26 or higher
- Dgraph v25.3.1

### Dgraph

Install Dgraph binaries as per the guidance at https://dgraph.io/docs/deploy/installation/download/, or see a summary below:

**Use Docker Image:**

This is the easier option for a quick start so you can play with Cogged or get involved in development:

```bash
docker pull dgraph/standalone:v25.3.1
docker run -it --rm --net=host dgraph/standalone:v25.3.1
```

**Installing binaries:**

Do this for a production environment:

```bash
export DGRAPHVERSION=v25.3.1
curl https://get.dgraph.io -sSf > dgraph.sh
sed -i 's/--lru_mb 2048//g' dgraph.sh
sed -i 's/grep -Fx/grep -F/g' dgraph.sh
bash dgraph.sh -y -s -v=$DGRAPHVERSION
```

## Build

```bash
./build.sh
```

The built binaries are output to the ./bin folder

## Test

Cogged has a layered test suite (see [CONTRIBUTING.md](./CONTRIBUTING.md) for detail):

```bash
# Fast, offline unit tests — no database or network required
go test ./...

# Integration tests — spin up an ephemeral Dgraph automatically via testcontainers
# (requires Docker running); no COGGED_TEST_DB_HOST needed
go test -tags=integration ./...
```

## Run/Usage

```bash
cd bin
./cogged -dh ${ip_of_prod_dgraph_db} -dp ${dgraph_rpc_port} -adduser system,sys
# note the password that it generated
./cogged -dh ${ip_of_prod_dgraph_db} -dp ${dgraph_rpc_port}
```

**Help**

```
./cogged --help
Usage of ./cogged:
  -adduser string
        Add a new Cogged user (supply 'username,role' as value, will generate and print a random password)
  -conf string
        Full filesystem path to config file (JSON)
  -dh string
        URL for Dgraph host eg. 10.1.2.3 (overrides config file)
  -dp string
        URL for Dgraph port eg. 9080 (overrides config file)
  -ip string
        Interface that Cogged binds to to listen for incoming connections (overrides config file)
  -p int
        TCP Port that Cogged listens on (overrides config file)
```