-- Partial-name search via ngram FULLTEXT (replaces slow LIKE '%token%' fallback).
-- Requires MySQL ngram_token_size=2 (see README infrastructure section).

ALTER TABLE sanctions_names
    ADD FULLTEXT INDEX sanctions_names_ngram_fulltext (
        first_name, middle_name, surname, single_string_name, original_script_name, entity_name
    ) WITH PARSER ngram;
