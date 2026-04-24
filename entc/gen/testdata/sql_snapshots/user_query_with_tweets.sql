driver.Query: query=SELECT `users`.`id`, `users`.`name` FROM `users` args=[<redacted>]
driver.Query: query=SELECT `t1`.`user_id`, `tweets`.`id`, `tweets`.`text` FROM `tweets` JOIN `user_tweets` AS `t1` ON `tweets`.`id` = `t1`.`tweet_id` WHERE `t1`.`user_id` IN (?) args=[<redacted>]
