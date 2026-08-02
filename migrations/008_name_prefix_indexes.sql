-- Records-list name filters use prefix matching (LIKE 'value%'), which a B-tree
-- index can serve as a range scan. A leading-wildcard LIKE '%value%' cannot use
-- an index at all and forced a full scan of sanctions_names per request.
--
-- A 32-character prefix keeps the index small while staying selective for any
-- realistic name search. Longer search terms remain correct because MySQL
-- rechecks the full column value after the index range scan.
--
-- INPLACE/LOCK=NONE keeps the table readable and writable while the index is
-- built. If your MySQL rejects it, drop the ALGORITHM/LOCK clause and expect the
-- table to be locked for the duration.

ALTER TABLE sanctions_names
    ADD INDEX idx_first_name_prefix (first_name(32)),
    ADD INDEX idx_surname_prefix (surname(32)),
    ALGORITHM=INPLACE, LOCK=NONE;
