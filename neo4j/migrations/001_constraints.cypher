// 001 - Identity constraints and lookup indexes.
// Every domain node carries the :Entity label plus its specific label
// (:Person, :Company, :BankAccount, :Transaction, :Device, :Document).
// A single uniqueness constraint on :Entity(id) guarantees ids are unique
// across all entity types and backs the MERGE statements used by the seed.
CREATE CONSTRAINT entity_id_unique IF NOT EXISTS
FOR (n:Entity) REQUIRE n.id IS UNIQUE;

CREATE INDEX person_name IF NOT EXISTS FOR (n:Person) ON (n.name);
CREATE INDEX company_name IF NOT EXISTS FOR (n:Company) ON (n.name);
CREATE INDEX transaction_timestamp IF NOT EXISTS FOR (n:Transaction) ON (n.timestamp);
CREATE INDEX document_created_at IF NOT EXISTS FOR (n:Document) ON (n.created_at);
