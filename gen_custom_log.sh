psql -td disgo -c "SELECT content FROM message WHERE chan_id = '$1' AND author_id = '$2' AND content != '';" > ~/markov/$2_custom
sed -i -r 's,<@.*?> ,,gm' ~/markov/$2_custom
sed -i -r '/^\/.*/d' ~/markov/$2_custom
sed -i -r 's,<@.*?>,,gm' ~/markov/$2_custom
sed -i -r '/^$/d' ~/markov/$2_custom
