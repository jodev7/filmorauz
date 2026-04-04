/**
 * Check if a movie is "New" (added within last 24 hours)
 */
export function getIsNew(createdAt: string | undefined | null | number): boolean {
  if (!createdAt) return false;
  
  // Handle different input types - createdAt could be:
  // 1. ISO string: "2024-01-01T00:00:00Z"
  // 2. Unix timestamp (seconds): "1704067200" or 1704067200
  // 3. Unix timestamp (milliseconds): "1704067200000" or 1704067200000
  let created: Date;
  
  if (typeof createdAt === 'number') {
    // Number could be seconds or milliseconds
    // If it's small (< 10 billion), it's likely seconds
    // If it's large (>= 10 billion), it's likely milliseconds
    if (createdAt < 10000000000) {
      created = new Date(createdAt * 1000); // Convert seconds to ms
    } else {
      created = new Date(createdAt);
    }
  } else if (typeof createdAt === 'string') {
    const trimmed = createdAt.trim();
    // Check if it's a numeric string
    if (/^\d+$/.test(trimmed)) {
      const num = parseInt(trimmed, 10);
      if (num < 10000000000) {
        created = new Date(num * 1000); // Convert seconds to ms
      } else {
        created = new Date(num);
      }
    } else {
      // Try to parse as ISO string or other date format
      created = new Date(trimmed);
    }
  } else {
    return false;
  }
  
  // Check if date is valid
  if (isNaN(created.getTime())) {
    return false;
  }
  
  const now = new Date();
  const diffMs = now.getTime() - created.getTime();
  
  // Invalid if date is in the future
  if (diffMs < 0) {
    return false;
  }
  
  const diffHours = diffMs / (1000 * 60 * 60);
  
  return diffHours < 24;
}

/**
 * Format relative time for when movie was added (always Uzbek)
 */
export function formatRelativeAddedTime(createdAt: string | undefined | null | number): string {
  if (!createdAt) return "";
  
  // Handle different input types - createdAt could be:
  // 1. ISO string: "2024-01-01T00:00:00Z"
  // 2. Unix timestamp (seconds): "1704067200" or 1704067200
  // 3. Unix timestamp (milliseconds): "1704067200000" or 1704067200000
  let created: Date;
  
  if (typeof createdAt === 'number') {
    // Number could be seconds or milliseconds
    // If it's small (< 10 billion), it's likely seconds
    // If it's large (>= 10 billion), it's likely milliseconds
    if (createdAt < 10000000000) {
      created = new Date(createdAt * 1000); // Convert seconds to ms
    } else {
      created = new Date(createdAt);
    }
  } else if (typeof createdAt === 'string') {
    const trimmed = createdAt.trim();
    // Check if it's a numeric string
    if (/^\d+$/.test(trimmed)) {
      const num = parseInt(trimmed, 10);
      if (num < 10000000000) {
        created = new Date(num * 1000); // Convert seconds to ms
      } else {
        created = new Date(num);
      }
    } else {
      // Try to parse as ISO string or other date format
      created = new Date(trimmed);
    }
  } else {
    return "";
  }
  
  // Check if date is valid
  if (isNaN(created.getTime())) {
    return "";
  }
  
  const now = new Date();
  const diffMs = now.getTime() - created.getTime();
  
  // Return empty if date is in the future or too far in the past (over 100 years = likely invalid)
  if (diffMs < 0 || diffMs > 100 * 365 * 24 * 60 * 60 * 1000) {
    return "";
  }
  
  const minutes = Math.floor(diffMs / (1000 * 60));
  const hours = Math.floor(diffMs / (1000 * 60 * 60));
  const days = Math.floor(diffMs / (1000 * 60 * 60 * 24));
  
  // Always use Uzbek translations
  const t = { minute: "daqiqa", minutes: "daqiqa", hour: "soat", hours: "soat", day: "kun", days: "kun" };
  
  if (minutes < 1) {
    return "hozir";
  }
  
  if (minutes < 60) {
    return minutes <= 1 
      ? `1 ${t.minute} avval`
      : `${minutes} ${t.minutes} avval`;
  }
  
  if (hours < 24) {
    return hours === 1 
      ? `1 ${t.hour} avval`
      : `${hours} ${t.hours} avval`;
  }
  
  if (days > 365) {
    return `${created.getFullYear()} yil`;
  }
  
  return days === 1 
    ? `1 ${t.day} avval`
    : `${days} ${t.days} avval`;
}
