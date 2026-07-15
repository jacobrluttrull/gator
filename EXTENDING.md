# Ideas for Extending gator

Not implemented yet — just notes for future work.

- Add sorting and filtering options to the `browse` command
- Add pagination to the `browse` command
- Add concurrency to the `agg` command so that it can fetch more frequently
- Add a `search` command that allows for fuzzy searching of posts
- Add bookmarking or liking posts
- Add a TUI that allows you to select a post in the terminal and view it in a more readable format (either in the terminal or open in a browser)
- Add an HTTP API (and authentication/authorization) that allows other users to interact with the service remotely
- Write a service manager that keeps the `agg` command running in the background and restarts it if it crashes
