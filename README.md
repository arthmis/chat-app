# Chatroom Application

This will eventually be a basic chatroom application that takes some inspiration
from Discord. It isn't meant to be a discord clone. It's only meant to be a learning 
experience for backend software, using NoSql(CassandraDB), and PostgreSQL.

## Warning
This Readme isn't complete because testing for Cassandra isn't complete or polished. 
So there will be some issues. 

## Building

The OS used is ubuntu 18.04.

You will need a [golang](https://golang.org/dl/) compiler, at least 1.14.4. 

You need to install at least PostgreSQL 10.12.
You can install Postgres with 
```
sudo apt update
sudo apt install postgresql postgresql-contrib
```
After installing, `psql` should be available as a command. 
You'll need to create a role using psql:
```
psql -c "CREATE ROLE chat WITH SUPERUSER CREATEDB LOGIN PASSWORD 'postgres';" -U postgres -h localhost
```
The prompted password is `postgres` when signing in with `postgres` user.
This first line creates a superuser role with the password `postgres` with the ability create databases.

Then create a database named `chat-app`:
```
psql -c 'CREATE DATABASE "chat-app";' -U postgres -h localhost
```

Then I change the owner of the database `chat-app` to be owned by the user `chat`. 
To do so execute:
```
psql -c 'ALTER DATABASE chat-app OWNER TO chat;' -U postgres -h localhost
```
If you run into any problems you can use this link to familiarize yourself with starting up [PostgreSQL](https://www.digitalocean.com/community/tutorials/how-to-install-and-use-postgresql-on-ubuntu-18-04). 
I got all my information from there. 

Now you should have the appropriate sql database the application is expecting.

Next you will have to install CassandraDB.
Use [Cassandra's](https://cassandra.apache.org/download/) installation page and follow the `installation from debian packages` instructions.
You need at least version 3.11.x. Make sure `nodetool status` works before proceeding. 

Now that Cassandra is up and running, you will have to create a keyspace in order to add the table to store messages:
```
cqlsh -e "CREATE KEYSPACE IF NOT EXISTS chatserver WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 3};"
```
Now that PostgreSQL and Cassandra are running, You'll need to create a file named `.env` in the root of the repository.
Add the below text to the file. They will serve as environment variables read in by a dotenv go package.
```
POSTGRES_PORT=5432
POSTGRES_HOST=localhost
POSTGRES_DB=chat-app
POSTGRES_USER=chat
POSTGRES_PASSWORD=postgres
SESSION_SECRET=sessionsecret
KEYSPACE=chatserver
PGTEST_PORT=5432
PGTEST_HOST=localhost
PGTEST_DB=testdb
PGTEST_USER=testdb
PGTEST_PASSWORD=postgres
```
With that everything should be ready. Go to the root of the repository and execute `go run` and the application should
be available on your browser at `localhost:8000`.


### Running Tests
You'll need to create a role using psql:
```
psql -c "CREATE ROLE testdb WITH SUPERUSER CREATEDB LOGIN PASSWORD 'postgres';" -U postgres -h localhost
```
Then create `testdb`:
```
psql -c 'CREATE DATABASE testdb;' -U postgres -h localhost
```
and then:
```
psql -c 'ALTER DATABASE testdb OWNER to testdb;' -U postgres -h localhost -p 5432 -w
```
This database will be used purely for running tests. 

Run all tests with `go test ./...`


