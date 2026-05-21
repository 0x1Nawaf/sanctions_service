ALTER TABLE sanctions_names DROP INDEX sanctions_names_fulltext;

ALTER TABLE sanctions_names
    ADD FULLTEXT INDEX sanctions_names_fulltext (first_name, middle_name, surname, single_string_name, original_script_name, entity_name);
