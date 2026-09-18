BEGIN;

-- 归属变更动作（一次变更一条，可回看）
-- 记录"谁在什么时候把哪块地转给谁"，以及变更前后分别挂在谁名下。
CREATE TABLE IF NOT EXISTS plot_ownership_transfer (
    id              BIGSERIAL PRIMARY KEY,
    plot_id         BIGINT NOT NULL REFERENCES plot(id),
    from_farm_id    BIGINT NOT NULL REFERENCES farm(id),
    to_farm_id      BIGINT NOT NULL REFERENCES farm(id),
    reason          VARCHAR(64) NOT NULL DEFAULT 'transfer', -- transfer / merge / split
    note            TEXT,
    operator        VARCHAR(128) NOT NULL,
    -- 变更时该地块名下的在挂批次 / 已发码情况，作为动作快照留存
    batch_count     INT NOT NULL DEFAULT 0,
    code_count      INT NOT NULL DEFAULT 0,
    transferred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_transfer_parties CHECK (
        from_farm_id <> to_farm_id AND plot_id IS NOT NULL
    )
);

COMMENT ON TABLE plot_ownership_transfer IS '地块归属变更动作流水，只追加不修改';

-- 地块归属时间线：每一段 [valid_from, valid_to) 表示该地块在这段时间归属 farm_id。
-- 最新一段 valid_to 为 NULL。历史回放按时间点落在哪个区间取 farm_id。
CREATE TABLE IF NOT EXISTS plot_ownership_period (
    id           BIGSERIAL PRIMARY KEY,
    plot_id      BIGINT NOT NULL REFERENCES plot(id),
    farm_id      BIGINT NOT NULL REFERENCES farm(id),
    valid_from   TIMESTAMPTZ NOT NULL,
    valid_to     TIMESTAMPTZ,
    transfer_id  BIGINT REFERENCES plot_ownership_transfer(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 同一块地最多只有一段"当前归属"（valid_to 为空），从库层面拦住两家同时认领。
CREATE UNIQUE INDEX IF NOT EXISTS uq_ownership_current
    ON plot_ownership_period(plot_id)
    WHERE valid_to IS NULL;

-- 同一时刻不允许两段归属区间重叠，从库层面拦住重复认领地。
ALTER TABLE plot_ownership_period
    ADD CONSTRAINT chk_ownership_range CHECK (valid_to IS NULL OR valid_to > valid_from);

CREATE INDEX IF NOT EXISTS idx_ownership_plot_time
    ON plot_ownership_period(plot_id, valid_from);

COMMENT ON TABLE plot_ownership_period IS '地块归属时间线，按区间记录历史归属';

-- 为存量地块回填初始归属段：从地块创建时起，归属当前 farm_id。
INSERT INTO plot_ownership_period (plot_id, farm_id, valid_from, transfer_id)
SELECT p.id, p.farm_id, p.created_at, NULL
FROM plot p
WHERE NOT EXISTS (
    SELECT 1 FROM plot_ownership_period pop WHERE pop.plot_id = p.id
);

CREATE INDEX IF NOT EXISTS idx_transfer_plot ON plot_ownership_transfer(plot_id);
CREATE INDEX IF NOT EXISTS idx_ownership_farm ON plot_ownership_period(farm_id);

COMMIT;
