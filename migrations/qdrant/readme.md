# 本目录下的文件不是应用程序自动执行的 migration 脚本，而是给运维或开发者手动执行的 Qdrant REST 片段。

# 用途：在 Qdrant 前端管理控制台（Dashboard Console）模块，创建 Collection 并建立 Payload 索引字段。

## 执行方式

1. 打开 Qdrant Dashboard -> Console
2. 按照代码块顺序，将文件中每个 HTTP 请求块**依次**粘贴执行
3. 每个请求返回 `"status": "ok"` 即为成功

## 注意事项

- Collection 已存在时重复执行会报错，属正常行为
- 向量维度 `size` 创建后不可更改，更换 Embedding 模型需删除重建
- Payload 索引可随时追加，不影响已有数据
- 空字符串 `""` 表示默认向量字段（单向量模式）
- `hnsw.max_indexing_threads` 设为 0 表示使用 Qdrant 自动检测的线程数；在容器化部署时需确认 CPU 配额，避免因线程争抢导致建索引变慢
- Payload 索引创建是异步操作，大批量数据写入前建议等待索引完成（通过 `GET /collections/dialogue_memory` 观察 `payload_schema` 中各字段的 `points` 计数是否与实际一致）
