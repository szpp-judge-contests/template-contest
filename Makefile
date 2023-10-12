.PHONY: check
check:
	@cd .cicd \
	&& go run ./check_task 

.PHONY: upload-tasks
upload-tasks:
	@cd .cicd \
	&& go run ./upload-tasks
