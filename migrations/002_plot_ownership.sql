BEGIN;

-- 地块归属变更动作记录（不可变，一次可回看的动作）
CREATE TABLE IF NOT EXISTS plot_transfer (
    id BIGSERIAL PRIMARY KEY,
    plot_id BIGINT NOT NULL REFERENCES plot(id),
    from_farm_id BIGINT NOT NULL REFERENCES farm(id),
    to_farm_id BIGINT NOT NULL REFERENCES farm(id),
    reason TEXT,
    operator VARCHAR(128) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 地块归属期：每行是一段 [valid_from, valid_to) 的归属区间，valid_to 为 NULL 表示当前归属
CREATE TABLE IF NOT EXISTS plot_ownership (
    id BIGSERIAL PRIMARY KEY,
    plot_id BIGINT NOT NULL REFERENCES plot(id),
    farm_id BIGINT NOT NULL REFERENCES farm(id),
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ,
    transfer_id BIGINT REFERENCES plot_transfer(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 一块地同一时刻最多一条有效归属期：拦住"两家同时认领"的 DB 兜底
CREATE UNIQUE INDEX IF NOT EXISTS idx_plot_ownership_current ON plot_ownership(plot_id) WHERE valid_to IS NULL;

CREATE INDEX IF NOT EXISTS idx_plot_ownership_plot ON plot_ownership(plot_id, valid_from);
CREATE INDEX IF NOT EXISTS idx_plot_transfer_plot ON plot_transfer(plot_id);

-- 存量地块补录初始归属期：从建地时间起挂在当前 farm_id 名下（幂等，可重跑）
INSERT INTO plot_ownership (plot_id, farm_id, valid_from, valid_to)
SELECT p.id, p.farm_id, p.created_at, NULL
FROM plot p
WHERE NOT EXISTS (SELECT 1 FROM plot_ownership po WHERE po.plot_id = p.id);

COMMIT;
