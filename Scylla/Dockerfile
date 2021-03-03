FROM scylladb/scylla

HEALTHCHECK --interval=30s --timeout=30s --start-period=30s --retries=3 CMD nodetool status
# ENTRYPOINT cqlsh -e "CREATE KEYSPACE IF NOT EXISTS chatserver WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};"
# ENTRYPOINT nodetool status

EXPOSE 7199
EXPOSE 9042
EXPOSE 7000
EXPOSE 7001
EXPOSE 9160
EXPOSE 10000