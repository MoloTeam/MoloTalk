package fixtures

// basicStatements — детерминированные данные набора basic. Идентификаторы и
// время фиксированы, пароли и сеансы отсутствуют.
var basicStatements = []string{
	`INSERT INTO users (id, identifier, display_name, created_at) VALUES
		('01ARZ3NDEKTSV4RRFFQ69G5FA1', 'alice@example.com', 'Alice', '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA2', 'bob@example.com',   'Bob',   '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA3', 'carol@example.com', 'Carol', '2026-10-19T09:00:00Z')`,

	`INSERT INTO channels (id, created_by_id, client_request_id, create_request_hash, kind, name, owner_id, direct_pair_key, created_at)
	 VALUES
		('01ARZ3NDEKTSV4RRFFQ69G5FA4', '01ARZ3NDEKTSV4RRFFQ69G5FA1', NULL, NULL,
		 'DIRECT', NULL, NULL, 'direct:alice:bob', '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA5', '01ARZ3NDEKTSV4RRFFQ69G5FA1',
		 '01ARZ3NDEKTSV4RRFFQ69G5FA6',
		 'f001ef788b1ad462a54cb64d2f21d25277db6acf58c57f6e3930f4528f7f126c',
		 'GROUP', 'Team', '01ARZ3NDEKTSV4RRFFQ69G5FA1', NULL, '2026-10-19T09:00:00Z')`,

	`INSERT INTO channel_members (channel_id, user_id, role, joined_at) VALUES
		('01ARZ3NDEKTSV4RRFFQ69G5FA4', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'MEMBER', '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA4', '01ARZ3NDEKTSV4RRFFQ69G5FA2', 'MEMBER', '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA5', '01ARZ3NDEKTSV4RRFFQ69G5FA1', 'MEMBER', '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA5', '01ARZ3NDEKTSV4RRFFQ69G5FA2', 'ADMIN',  '2026-10-19T09:00:00Z'),
		('01ARZ3NDEKTSV4RRFFQ69G5FA5', '01ARZ3NDEKTSV4RRFFQ69G5FA3', 'MEMBER', '2026-10-19T09:00:00Z')`,
}

var sets = map[Set][]string{
	Basic: basicStatements,
}
