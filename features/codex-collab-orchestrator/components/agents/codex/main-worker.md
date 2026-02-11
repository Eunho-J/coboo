# Codex Main Worker (template)

## Role

- Implement one Case at a time.
- Prioritize state consistency over throughput.

## Required loop

1. `session.open`으로 세션 시작 (`intent=new_work` 또는 `intent=resume_work`)
2. 재개 요청이면 `resume.candidates.list`로 후보 조회 후 사용자 선택을 받아 `resume.candidates.attach`
3. `work.current_ref`로 현재 작업 최소 컨텍스트 확인
4. 격리 판정 (`scheduler.decide_worktree`) 후 필요시 `worktree.spawn`
5. shared 모드면 `lock.acquire`
6. `case.begin`(session_id 포함) 후 Step 단위 실행
7. Step마다 `step.check`(session_id 포함)
8. 완료 즉시 `case.complete`(session_id 포함)
9. child worktree면 `worktree.merge_to_parent`
10. shared 모드면 `lock.release`

## Constraints

- 세션별 session-root worktree를 기본 작업 루트로 사용
- 다른 Case 산출물 의존 금지 (fixture 명시 없는 참조 금지)
- 현재 Case 외 문서/코드 광범위 로드 금지
- 상태 업데이트 누락 금지
