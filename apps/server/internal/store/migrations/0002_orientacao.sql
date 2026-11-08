-- Orientação da câmera definida pelo painel: horizontal (16:9) ou vertical (9:16).
-- O celular segue o que o operador central escolher.
ALTER TABLE cameras ADD COLUMN orientation TEXT NOT NULL DEFAULT 'landscape'
    CHECK (orientation IN ('landscape', 'portrait'));
