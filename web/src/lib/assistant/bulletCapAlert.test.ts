import { describe, expect, it } from 'vitest';
import { bulletCapUserMessage } from './bulletCapAlert';

describe('bulletCapUserMessage', () => {
  it('returns null when the tool result is unrelated', () => {
    expect(bulletCapUserMessage('{"error":"needs evidence_id"}')).toBeNull();
    expect(bulletCapUserMessage(undefined)).toBeNull();
  });

  it('surfaces a safe banner from the backend ErrListCap envelope', () => {
    const raw =
      '{"error":"cvedit: this edit would drop existing CV content: bullet_cap: Staff Engineer at Neon already has 20 bullets (the maximum). The edit was not applied and no existing bullets were deleted. Set an existing bullet or remove one before inserting"}';
    const got = bulletCapUserMessage(raw);
    expect(got).toContain('Staff Engineer at Neon');
    expect(got).toContain('Your existing bullets were kept');
    expect(got).not.toContain('bullet_cap');
    expect(got).not.toContain('Set an existing');
    expect(got).not.toContain('cvedit:');
  });
});
