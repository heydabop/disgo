psql -td disgo -c "SELECT content FROM message WHERE chan_id = '$1' AND content != '';" > ~/markov/chan_$1_custom
sed -i -r 's,<@.*?> ,,gm' ~/markov/chan_$1_custom
sed -i -r '/^\/.*/d' ~/markov/chan_$1_custom
sed -i -r 's,<@.*?>,,gm' ~/markov/chan_$1_custom
sed -i -r '/^$/d' ~/markov/chan_$1_custom
