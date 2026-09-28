package main

// errors.go —— 本示例的错误 Op，写法见 docs/CODE_STANDARDS.md §5.1。

const (
	opNewClient = "fidelity.new_client"
	opFetchPage = "fidelity.fetch_page" // step 1：GET /
	opSubmit    = "fidelity.submit"     // step 2：POST /api/submit
)
