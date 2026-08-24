-- Reverse the artists refresh: drop added entries and restore the lowercase name.
UPDATE gemfactory.artists SET name = 'tuide' WHERE name = 'TUIDE';

DELETE FROM gemfactory.artists WHERE name IN (
    'ALICE SYNDROME', 'AWU', 'DIA', 'EVERGLOW', 'HEART OF WOMAN', 'I.O.I', 'Keyveatz',
    'LEE CHAEYEON', 'LEE YOUNGJI', 'MIMI', 'OURBIRTHDAY', 'UNCHILD',
    'BIGBANG', 'EXO', 'J.Y. Park', 'LNGSHOT', 'NCT 127', 'Picheolin',
    'TOMORROW X TOGETHER', 'XLOV', 'YEONJUN'
);
