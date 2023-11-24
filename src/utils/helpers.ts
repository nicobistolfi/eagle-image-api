function toBoolean(value: any): boolean {
  return value === true || value === 'true' || value === 1 || value === '1';
}

export {
  // eslint-disable-next-line import/prefer-default-export
  toBoolean,
};
