DROP INDEX IF EXISTS idx_post_comments_author;
DROP INDEX IF EXISTS idx_post_comments_parent_created;
DROP INDEX IF EXISTS idx_post_comments_post_created;
DROP TABLE IF EXISTS post_comments;
ALTER TABLE posts DROP COLUMN IF EXISTS comment_count;
