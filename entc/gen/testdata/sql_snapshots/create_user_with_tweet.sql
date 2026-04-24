driver.Query: query=INSERT INTO `tweets` (`text`) VALUES (?) RETURNING `id` args=[<redacted>]
driver.Tx(<id>): started
Tx(<id>).Query: query=INSERT INTO `users` (`name`) VALUES (?) RETURNING `id` args=[<redacted>]
Tx(<id>).Exec: query=INSERT INTO `user_tweets` (`user_id`, `tweet_id`, `created_at`) VALUES (?, ?, ?) args=[<redacted>]
Tx(<id>): committed
