ALTER TABLE consents
    ADD CONSTRAINT consents_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
