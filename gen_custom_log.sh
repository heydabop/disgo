sqlite3 sqlite.db "select Content from Message where ChanId = '$1' and AuthorId == '$2' and Content != '';" > ~/markov/$2_custom
sed -i -r 's,<@.*?> ,,gm' ~/markov/$2_custom
sed -i -r '/^\/.*/d' ~/markov/$2_custom
sed -i -r 's,<@.*?>,,gm' ~/markov/$2_custom
sed -i -r '/^$/d' ~/markov/$2_custom
